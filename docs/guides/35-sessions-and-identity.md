# Sessions and identity

This chapter reads the signed-in user off a request and then explains the machinery behind
that read: what the session context carries, why the state lives in a database row, how CSRF
tokens and flash messages are stored, and what `Common\Identity` exists for.

## Getting the signed-in user

Two calls:

```php
$context = $session->current();
$user = $context["is_authenticated"] ? $users->find($context["user_id"]) : false;
```

It is two calls because the session carries an id and the user store carries the record. A
session that loaded the whole row would go stale the moment that record changed: rename the
account, revoke its administrator flag or deactivate it, and every open session would still
answer from the copy it took when the cookie was minted.

`Common\Session::current()` always returns an array, so no caller tests for false and no
caller has two shapes to read. A request with no cookie, a cookie naming a revoked session and
a cookie naming an expired one all resolve to the same anonymous context, because the page
each of them lands on is the login form.

The context has exactly six keys:

| Key                | Anonymous | Signed in                                          |
|--------------------|-----------|----------------------------------------------------|
| `is_authenticated` | `false`   | `true`                                             |
| `session_id`       | `""`      | the `user_session.id`, 26 characters               |
| `user_id`          | `""`      | the `user.id`, 26 characters                       |
| `csrf_token`       | `""`      | 32 hex characters                                  |
| `remote_addr`      | `""`      | what `$_SERVER["REMOTE_ADDR"]` said at sign-in     |
| `user_agent`       | `""`      | what `$_SERVER["HTTP_USER_AGENT"]` said at sign-in |

The session token is not among them. The context is what a template renders, and a token that
reached a template is a session waiting to be copied out of a page cache or a referrer
header. `Common\Session` is the only object that ever holds it.

Most routes reach the context through `$context = require_login($session);`, the guard in
`bootstrap.php` that redirects an anonymous request to the login form and returns the context
otherwise.

## Wiring the session

`bootstrap.php` builds it:

```php
$session = new Common\Session(
	new \Session\Manager(new \Session\Storage\Disk()),
	$session_store,
	$config->session_ttl()
);
$csrf = new Common\Csrf($session);
$flash = new Common\Flash($session, $session_store);
```

`Session\Manager` is a host binding. It takes a storage object, either `Session\Storage\Disk`,
which keeps one file per session under the operating system's temporary directory, or
`Session\Storage\Memory`, which loses them when the process exits. Read `stdlib/session/` for
the implementation and `docs/design.md` for the decisions behind it.

Write `\Session\Manager` with the leading backslash. Inside `namespace Common;` the bare name
resolves to `Common\Session\Manager`, and this package declares a class `Common\Session`, so
the bare spelling fails with `undefined class`.

The manager holds exactly one opaque string per browser, through three methods:
`start($value)`, `get()` and `valid()`. `start()` mints a fresh 32 byte identifier from Go's
`crypto/rand`, stores `$value` against it and stages the cookie itself:

```text
Set-Cookie: session=524ad2abf1f5368f899d8c89042c4e01ab07257399b9fb8d21d001746efdcbec; Path=/; HttpOnly; SameSite=Lax
```

`HttpOnly`, `Path=/` and `SameSite=Lax` are fixed in Go. A script cannot weaken them and cannot
choose the cookie value. That also settles session fixation without a line of PHP, since
`start()` never reuses an id.

There is no `$_SESSION` and no `session_start()` in this runtime. `$_COOKIE`
is the read side and is populated per request. A cookie goes out through
`setcookie()`, or through
`header("Set-Cookie: ...", false)`, which is the string `setcookie()` would have formatted.

## Why session state lives in a row

The manager stores one immutable string and `start()` always mints a new cookie. There is no
way to change the value of an existing cookie and no way to delete one. So mutable state
cannot live in the cookie: the string it stores is a token, and everything about a session
that can change is a column on the row that token names.

```sql
CREATE TABLE IF NOT EXISTS user_session (
	id CHAR(26) PRIMARY KEY NOT NULL,
	token VARCHAR(32) NOT NULL,
	user_id CHAR(26) NOT NULL,
	csrf_token VARCHAR(32) NOT NULL,
	flash TEXT NOT NULL DEFAULT '',
	remote_addr VARCHAR(45) NOT NULL DEFAULT '',
	user_agent VARCHAR(255) NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL,
	revoked_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS uidx_user_session_token ON user_session(token);
CREATE INDEX IF NOT EXISTS idx_user_session_user_id ON user_session(user_id);
CREATE INDEX IF NOT EXISTS idx_user_session_expires_at ON user_session(expires_at);
```

That is `demos/common-phpscript/schema/user_session.up.sql`. Rotating a CSRF token, leaving a
flash message, recording a last-seen time and signing out are `UPDATE` statements against this
row, and none of them touches the cookie.

Two values address a session. `SessionStore::find()` takes the token, which is what arrived in
the cookie; every other method takes `id`, because the token is a bearer credential that stays
out of the context. `Common\Session::current()` turns one into the other once per request and
caches the answer, since `Common\Csrf`, `Common\Flash` and `Common\SessionIdentity` all ask.

## Signing out

```php
require_csrf($html, $csrf);
$session->revoke();
redirect_to("/admin/login");
```

`revoke()` marks the row revoked and then calls `$this->manager->start("")`. Both halves are
needed for the same reason: there is no way to delete a cookie. `start("")` mints a fresh
cookie whose stored payload is the empty string, which names no row, so the next request
resolves to anonymous. Watch the logout response with curl and a new random cookie value comes
back rather than an expiry.

The row survives the sign-out, so a cookie replayed afterwards is answered by a row that says
no. `find()` excludes revoked and expired rows in its `WHERE` clause, and the row stays until
the pruner takes it.

`revoke_all($user_id)` ends every session a user has open, which is the answer to a stolen
password and does not depend on the current request holding any of them. The route that
changes a password calls it, then starts a fresh session when the account being changed is the
one making the request.

## Where the tokens come from

PHP reaches no CSPRNG in this runtime. `random_bytes`, `rand`, `md5`, `sha1`, `hash` and
`hash_hmac` are all absent. The only generator in range belongs to the database, so tokens
are minted in SQL through `Common\Store\Connection`:

```php
public function token()
{
	$row = $this->db->get("SELECT " . $this->random_expr(32) . " AS token");

	return (string)$row["token"];
}

public function random_expr($length)
{
	if ($this->driver === "mysql") {
		return "lower(substr(replace(uuid(), '-', ''), 1, " . (int)$length . "))";
	}

	if ($this->driver === "postgres") {
		return "substr(md5(random()::text || clock_timestamp()::text), 1, " . (int)$length . ")";
	}

	return "lower(substr(hex(randomblob(" . (int)$length . ")), 1, " . (int)$length . "))";
}
```

MySQL's `UUID()` is 32 hex characters once the dashes are removed, which bounds this at 32 on
that driver. Nothing asks for more.

The credential an attacker would have to produce is the cookie, which is 32 bytes from the
host's generator. The row token only has to be unguessable enough that a session file
surviving a rebuilt database cannot land on a recycled row.

## No timestamps in the context

`created_at`, `last_seen_at` and `expires_at` are on the row and kept off the context. A
`DATETIME` column arrives from the driver as a Go time object: `echo` prints the empty string,
`__toString` is never called, and `date()` takes a timestamp rather than that object, so nothing turns it into anything
else.

So expiry is written and compared in SQL and never projected into a template.
`Connection::now_expr($offset_seconds)` gives the server's expression for the current time
plus an offset, `create()` uses it for `expires_at`, and `find()` uses it again:

```sql
SELECT id, user_id, csrf_token, remote_addr, user_agent
FROM user_session
WHERE token = ? AND revoked_at IS NULL AND expires_at > datetime('now', '0 seconds')
```

Comparing in the `WHERE` clause is also the only way expiry stays correct when the web
process and the database disagree about the clock.

## Checking that a POST came from your page

`Common\Csrf::token()` returns the value to embed in a form, `""` when the request is
anonymous. `Common\Csrf::check($sent)` reports whether `$sent` is this session's token.

The token is the `csrf_token` column on the session row, and it is a different value from the
cookie. That separation is the point: a token embedded in a page is present in the page
source, so it must not be the credential that authenticates the request carrying it.

`check()` refuses the empty string before it compares, so an anonymous request cannot pass by
posting an empty field. The comparison is `===` rather than constant-time, because
the tokens are compared whole rather than by prefix; a cross-site POST cannot read the response, so it has no oracle
to time.

Every POST route starts with the guard from `bootstrap.php`:

```php
function require_csrf($html, $csrf)
{
	$sent = isset($_POST["csrf_token"]) ? $_POST["csrf_token"] : "";
	if (!$csrf->check($sent)) {
		fail($html, 403, "The form expired. Reload and try again.");
	}
}
```

And every form carries the hidden field. `csrf_token` is in `$frame`, so a template rendered
inside the layout already has it:

```html
<form class="card" method="post" action="/admin/group">
	<input type="hidden" name="csrf_token" value="{csrf_token}">
	<input id="name" name="name" required autofocus>
	<button type="submit">Create group</button>
</form>
```

The login form is the one place that needs a token before there is a session.
`Common\Session::ensure($remote_addr, $user_agent)` creates a row for an anonymous visitor so
that the form has a token to carry. See
[Users and authentication](30-users-and-authentication.md) for `ensure()` and the sign-in
flow it belongs to.

## Leaving a message across a redirect

`Common\Flash::set($message)` and `Common\Flash::take()`. The message is the `flash` column on
the session row. A second cookie is not available, and keeping the text out of the response
keeps it out of anything the browser stores, which matters when the message names what was
deleted. There is one slot rather than a queue: a request that sets two messages keeps the
second, which is all a redirect can display anyway.

Taking clears. That is why the frame does not carry the flash and a route asks for it
explicitly: a POST that re-rendered its own form instead of redirecting would consume a
message it never displayed. The pattern is a POST that sets and redirects:

```php
$id = $groups->create($name, $description);
$flash->set("Created " . $name . ".");
redirect_to("/admin/group/" . $id);
```

and a GET that takes:

```php
echo page($html, $frame, "admin-groups-list.tpl", array(
	"groups" => $groups->all(),
	"message" => $flash->take(),
));
```

Load that list twice and the notice appears once.

`set()` on an anonymous request returns false and drops the message: there is no row to store
it on, and leaving a note for a session about to be replaced by the login form is not worth
failing a request over.

## The identity seam

`Common\Rules` needs to know who the request is from before it builds a grant query. It asks
through an interface with two methods:

```php
interface Identity
{
	/** user_id returns the signed-in user's id, or "" when the request is anonymous. */
	public function user_id();

	/** is_admin reports whether $user_id bypasses the grant query. */
	public function is_admin($user_id);
}
```

`Common\SessionIdentity` satisfies it from the session. `user_id()` reads the context;
`is_admin()` asks `Common\Users` once per id and caches the answer for the life of the object,
which is one request. `Common\Rules` asks before every check, so a page with thirty checks
would otherwise pay for thirty lookups on the same id. An anonymous id is answered without
touching the store.

The seam is two methods wide because that keeps a permission fixture cheap.
`Common\Mock\MemoryIdentity` is a constructor taking a user id and an administrator flag, so
`src/Rules.phpt` exercises the whole permission engine with no user component and no database.
[Groups and permissions](40-groups-and-permissions.md) builds on this.

## Pruning expired sessions

`user_session` is the one table in the package that no request path deletes from. Sign-out
marks a row, and an expired row is never matched again. A scheduled job clears both:

```php
<?php

// @schedule hourly

include "bootstrap.php";

echo "session-prune: removed " . $session->prune() . " sessions\n";
```

`Common\Session::prune()` deletes rows that are expired or revoked and returns how many it
removed. The job includes the same `bootstrap.php` the route files include, so there is no
second composition root and no CLI-only wiring to keep in step.

Two limits. The cookie storage expires independently of the row table, so a deployment that
also wants the files on disk cleaned calls `Session\Storage\Disk::prune($max_age)` as a second
step. And the scheduler's overlap lock is a mutex inside one process: two server processes
over one tree both tick, each on its own clock. That costs nothing here because the delete is
idempotent, but the lock is not distributed, and a job that is not idempotent needs one of its
own.

Next: [Groups and permissions](40-groups-and-permissions.md).
