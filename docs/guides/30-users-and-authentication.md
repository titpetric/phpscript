# Users and authentication

This chapter gives the application accounts. You install `titpetric/phpscript-common`, create
the `user` table, read and write rows through `Common\Users`, hash passwords with
`Common\Auth`, and wire the three routes that make up a sign-in: the form, the login POST and
the logout POST. At the end a visitor signs in at `/admin/login`, and every chapter after this
one has a user id to work with.

## Install the package

```bash
composer require titpetric/phpscript-common
cp -n vendor/titpetric/phpscript-common/routes/*.php routes/
```

The second line is not a convenience. The annotation scanner walks the served tree and skips
`vendor/`, so a file carrying `// @route` under `vendor/titpetric/phpscript-common/routes/` is
never registered and never served. A composer package cannot publish routes; the application
copies them into its own `routes/`, where the scanner reads them. `phpscript list ./...`
prints one row per registered route, and a missing row is a file that was not copied.

`-n` never overwrites, so a route file you have edited survives an upgrade and a route added
by a later release lands beside it.

Templates need no copying. minitpl resolves a template through its search path, an ordinary
file read that has nothing to do with the scanner. `bootstrap.php` lists the application's
directory first and the package's second, earliest wins, and you override one template by
putting a file of the same name in your own directory:

```php
"template_paths" => array("templates/", "vendor/titpetric/phpscript-common/templates/"),
```

## Create the user table

`schema/user.up.sql`, applied by `Database\Migrate` as [Databases and
migrations](15-databases-and-migrations.md) describes:

```sql
CREATE TABLE IF NOT EXISTS user (
	id CHAR(26) PRIMARY KEY NOT NULL,
	username VARCHAR(64) NOT NULL,
	email VARCHAR(255) NOT NULL DEFAULT '',
	password VARCHAR(255) NOT NULL DEFAULT '',
	is_admin TINYINT NOT NULL DEFAULT 0,
	is_active TINYINT NOT NULL DEFAULT 1,
	properties TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS uidx_user_username ON user(username);
CREATE INDEX IF NOT EXISTS idx_user_email ON user(email);
```

What is missing is the design. There is no `salt` column, because bcrypt encodes a 128 bit
salt inside the hash string, and no `algo` column, because the `$2y$` prefix and the two digit
cost are in that same string and `password_get_info()` parses them back out. There is no
`password_cost` column either: the cost you want lives in `bootstrap.php`, the cost you have
lives in the hash, and `password_needs_rehash()` compares the two.

`id` is a ULID minted by the application, and the schema has no foreign keys, so removing a
user removes its `user_group_member` and `user_session` rows as an application step.

## Read and write users with `Common\Users`

`Common\Users` is the whole read and write surface for the table. Everything it needs from
storage sits behind a `UserStore` interface, so a fixture runs it with no database.

```php
Common\Users::find($id)                    // decorated row, or false
Common\Users::find_many(array $ids)        // rows keyed by id, missing ids absent
Common\Users::find_by_username($username)  // matches username or email, or false
Common\Users::exists($username)            // the id holding the name, or false
Common\Users::create(array $fields)        // returns the new id
Common\Users::update($id, array $fields)   // returns rows changed
Common\Users::remove($id)                  // returns rows removed
Common\Users::is_admin($id)                // bool
Common\Users::all()                        // every row, ordered by username
Common\Users::count()                      // how many users there are
```

`create()` and `update()` store a `password` value exactly as given. Passwords are not this
class's concern: `Common\Auth` owns hashing, and `Users` never hashes, compares or rehashes
anything. `routes/admin-users-create.php` shows the split.

```php
$hash = $auth->hash($password);
if ($hash === false) {
	fail($html, 422, "That password is longer than 72 bytes.");
}

$id = $users->create(array(
	"username" => $username,
	"email" => isset($_POST["email"]) ? trim($_POST["email"]) : "",
	"password" => $hash,
	"is_admin" => isset($_POST["is_admin"]) ? 1 : 0,
	"is_active" => isset($_POST["is_active"]) ? 1 : 0,
));
```

`create()` refuses a taken name and a missing one, and throws rather than returning false; a
route that wants 409 and the name back checks `exists()` itself first. The unique index is
what decides a race. `update($id, array("password" => $hash))` takes the same field names and
returns rows changed, so zero means the id matched nothing or the values were already stored.

`is_admin()` goes straight to the store instead of reading the row through `find()`, because
`Common\Rules` calls it before every grant query and needs one column. `count()` is a count
query and not `count(all())`, which is what makes it usable as the dashboard tile
`array("label" => "Users", "value" => $this->users->count())`.

## Decorate a row without a subclass

A deployment usually wants extra fields on a user without adding columns. The legacy shape for
that was an overridable `prepare()` method, which forces a subclass to exist before the class
can be used at all. `Common\Users` takes a decorator as its second constructor argument
instead, and `PropertiesDecorator` is the one implementation the application ships: it decodes
the `properties` JSON column and merges it over the row, decoded keys winning, with the
`properties` key removed so nothing can be read two ways.

```php
$store = new Common\Mock\MemoryUserStore();
$id = $store->insert(array("username" => "tit", "properties" => '{"nick":"tp"}'));

$plain = new Common\Users($store);
$wide = new Common\Users($store, new Common\PropertiesDecorator());

echo $plain->find($id)["properties"], "\n";
echo $wide->find($id)["nick"], "\n";
```

```text
{"nick":"tp"}
tp
```

A caller that wants stored rows passes no decorator, which is why there is no `$raw` flag on
the read methods: the shape of a row is decided once, where the object is built. A column that
is null, empty or not valid JSON leaves the row untouched, because this is a widening step
rather than a validation step.

## Hash a password

Four functions are bound, and they are the only password cryptography in the runtime.

| Function                                                   | What it does                                |
|------------------------------------------------------------|---------------------------------------------|
| `password_hash($password, PASSWORD_DEFAULT, $options)`     | returns a 60 byte `$2y$` string             |
| `password_verify($password, $hash)`                        | bool, false for an empty or malformed hash  |
| `password_needs_rehash($hash, PASSWORD_DEFAULT, $options)` | bool, true for anything bcrypt cannot parse |
| `password_get_info($hash)`                                 | `algo`, `algoName`, `options["cost"]`       |

bcrypt is the only algorithm, so `PASSWORD_DEFAULT` and `PASSWORD_BCRYPT` are the same value.
The digests are there too - `md5`, `sha1`, `hash`, `hash_hmac` and `random_bytes` - but bcrypt is the only password scheme, which is why the binding is
this narrow.

`Common\Auth::hash()` is the whole of the application's hashing:

```php
public function hash($password)
{
	if (strlen($password) > 72) {
		return false;
	}

	return password_hash($password, PASSWORD_DEFAULT, array("cost" => $this->cost));
}
```

bcrypt refuses a password over 72 bytes with an error rather than truncating it, so the length
is checked here and the caller is told no. Truncating would silently accept every later
character, and two passwords sharing their first 72 bytes would sign in as each other. Every
caller checks the false. The cost comes from the composition root, `"password_cost" => 12` in
`bootstrap.php`, and is recorded inside each hash.

## Sign a user in

`Common\Auth::attempt($username, $password)` returns a user id or false, and it does not touch
the session; the caller starts one. That separation buys three things. `Session\Manager::start()`
needs an HTTP request context, so a class that minted a cookie could not be exercised from a
fixture. A CLI script creating the first administrator checks a credential without issuing a
cookie. A destructive action asks for the password again without re-authenticating anybody.

A missing user costs the same as a wrong password. `password_verify()` against an empty hash
runs a decoy derivation inside the runtime rather than returning early, so response time does
not answer "does this account exist", and the `is_active` test comes after the verify for the
same reason.

`routes/admin-auth-login.php` is the whole login handler:

```php
<?php

// @route POST /admin/login

use Common\Render\Problem;
use Common\SafeRedirect;

include "bootstrap.php";

try {
	require_csrf($html, $csrf);

	$username = isset($_POST["username"]) ? trim($_POST["username"]) : "";
	$password = isset($_POST["password"]) ? $_POST["password"] : "";
	$next = SafeRedirect::path(isset($_POST["next"]) ? $_POST["next"] : "", "/admin");

	$user_id = $auth->attempt($username, $password);
	if ($user_id === false) {
		redirect_to("/admin/login?failed=1&next=" . rawurlencode($next));
	}

	$row = $users->find($user_id);
	if ($row !== false && isset($row["password"]) && $auth->needs_rehash($row["password"])) {
		$rehashed = $auth->hash($password);
		if ($rehashed !== false) {
			$users->update($user_id, array("password" => $rehashed));
		}
	}

	$remote_addr = isset($_SERVER["REMOTE_ADDR"]) ? $_SERVER["REMOTE_ADDR"] : "";
	$user_agent = isset($_SERVER["HTTP_USER_AGENT"]) ? $_SERVER["HTTP_USER_AGENT"] : "";
	$session->start($user_id, $remote_addr, $user_agent);

	$flash->set("Signed in.");

	redirect_to($next);
} catch (Exception $e) {
	$problem = Problem::of($e);
	http_response_code(Problem::status($e));
	echo $html->get("_exception.tpl", $problem["error"]);
}
```

Read it in order. `require_csrf()` runs first, so a POST with no token or a stale one ends in
403 before a password is verified. The posted `next` goes through `Common\SafeRedirect::path()`
because the visitor chose it. The failure branch does not report which half was wrong: "no
such user" and "wrong password" are the same answer here, because telling them apart is how an
account list gets enumerated.

Then the rehash. A successful attempt is the one moment when both the plaintext and the stored
hash exist in the same request, so it is the only place a hash can be re-derived at a new
cost. Raising `password_cost` in `bootstrap.php` rewrites each stored hash the first time its
owner signs in, with no migration and no forced reset. `needs_rehash()` reports true for
anything bcrypt cannot parse, so a leftover from another scheme is an upgrade rather than a
valid hash.

`$session->start()` mints a fresh cookie and a fresh CSRF token, so session fixation needs no
separate handling: an id planted before the sign-in is not the one that exists after it. The
flash survives on the session row, and the redirect stops a reload from repeating the POST.

Drive it. `demos/common-phpscript` serves with `phpscript -f config.yml server .`, and the
seeded account is `admin` / `admin`:

```bash
curl -s -c jar.txt http://127.0.0.1:8080/admin/login -o login.html
TOKEN=$(grep -o 'name="csrf_token" value="[^"]*"' login.html | sed 's/.*value="//;s/"//')

curl -s -b jar.txt -c jar.txt -o /dev/null -w "%{http_code} %{redirect_url}\n" \
	-d "csrf_token=$TOKEN" -d "username=admin" -d "password=nope" \
	http://127.0.0.1:8080/admin/login

curl -s -b jar.txt -c jar.txt -o /dev/null -w "%{http_code} %{redirect_url}\n" \
	-d "csrf_token=$TOKEN" -d "username=admin" -d "password=admin" \
	http://127.0.0.1:8080/admin/login
```

```text
302 http://127.0.0.1:8080/admin/login?failed=1&next=%2Fadmin
302 http://127.0.0.1:8080/admin
```

A POST with no `csrf_token` at all answers 403 and never reaches `attempt()`.

## Sign a user out

```php
<?php

// @route POST /admin/logout

use Common\Render\Problem;

include "bootstrap.php";

try {
	require_csrf($html, $csrf);

	$session->revoke();

	redirect_to("/admin/login");
} catch (Exception $e) {
	$problem = Problem::of($e);
	http_response_code(Problem::status($e));
	echo $html->get("_exception.tpl", $problem["error"]);
}
```

`revoke()` does two things. It marks the `user_session` row revoked rather than deleting it,
so a cookie replayed after the sign-out meets a row that says no instead of nothing at all.
Then it starts the manager with the empty string, because there is no way to delete a cookie
here: `Session\Manager` has no destroy, so `start("")` mints a
fresh cookie whose payload names no row and the next request resolves to anonymous.

Signing out is a POST because it is a write, and it carries a CSRF token for the same reason
every other write does.

## Render the login form, and give it a token

A signed-out visitor has no session row and therefore no CSRF token, so the token rendered
into the login form would be the empty string, and `Csrf::check()` refuses the empty string.
The login POST could never pass its own check.

`Common\Session::ensure($remote_addr, $user_agent)` is the answer. It returns the current
context when a token is already there, and creates an anonymous row when it is not. Only the
login form route calls it, because it writes: calling it from `current()` would give every
crawler that touches a public page a row, which is a table an anonymous client can grow. The
row carries no user id, and `start()` replaces it rather than promoting it.

```php
<?php

// @route GET /admin/login

use Common\Render\Problem;
use Common\SafeRedirect;

include "bootstrap.php";

try {
	$remote_addr = isset($_SERVER["REMOTE_ADDR"]) ? $_SERVER["REMOTE_ADDR"] : "";
	$user_agent = isset($_SERVER["HTTP_USER_AGENT"]) ? $_SERVER["HTTP_USER_AGENT"] : "";
	$context = $session->ensure($remote_addr, $user_agent);

	if ($context["is_authenticated"]) {
		redirect_to("/admin");
	}

	$next = SafeRedirect::path(isset($_GET["next"]) ? $_GET["next"] : "", "/admin");

	echo page($html, $frame, "admin-auth-form.tpl", array(
		"csrf_token" => $context["csrf_token"],
		"identity" => $context,
		"next" => $next,
		"failed" => isset($_GET["failed"]),
		"message" => $flash->take(),
	));
} catch (Exception $e) {
	$problem = Problem::of($e);
	http_response_code(Problem::status($e));
	echo $html->get("_exception.tpl", $problem["error"]);
}
```

The `csrf_token` entry overrides the frame's copy on purpose. `$frame` was built at the top of
`bootstrap.php`, before `ensure()` ran, so it holds the empty token of the anonymous context.

## Seed the first administrator

An empty `user` table has nobody who can sign in and create the first account.
`demos/common-phpscript/migrate.php` carries `@startup`, so the server runs it to completion
before it listens, and it ends with a seed:

```php
if ($users->count() > 0) {
	echo "migrate: " . $users->count() . " users, no seed needed\n";
	return;
}

$hash = $auth->hash($seed_password);
if ($hash === false) {
	echo "migrate: the seed password is longer than bcrypt accepts, no user created\n";
	return;
}

$seed_id = $users->create(array(
	"username" => $seed_user,
	"email" => "",
	"password" => $hash,
	"is_admin" => 1,
	"is_active" => 1,
));
```

`$seed_user` and `$seed_password` come from `COMMON_ADMIN_USER` and `COMMON_ADMIN_PASSWORD`
and default to `admin` and `admin`. The `count()` guard is what makes this safe to leave in
place: an empty table is the only case it fires in, and re-running it on a populated one would
either fail on the unique index or reset a password the operator had changed. `is_admin` short
circuits every permission check, which is what lets the first account reach the permission
screen and grant everything else.

```text
migrate: schema applied
migrate: 3 modules registered
migrate: seeded administrator admin with id c1c516f553984ec158220ba772
migrate: sign in at /admin/login and change the password
```

## What this does not do

There is no rate limit on failed sign-ins in the shipped code. A guard written in PHP at the
top of the route file runs after the request has already paid for reading the body, building a
runtime, registering the standard library and parsing the file, so it shortens the response
without avoiding the work. The design for doing it in Go, before a PHP runtime exists, is
[proposal-annotation-ratelimit.md](../../demos/common-report/proposal-annotation-ratelimit.md).
Counting failures in the `user` table is not the answer either, because that turns every wrong
guess into a write to the table under attack.

A legacy table of `md5` passwords is not upgraded on login by `password_verify()`, which
returns false against an `md5` digest so the account never reaches the rehash step. `md5()` is
bound, so a two-step check can be written - verify the legacy digest, then write a bcrypt hash
- but it puts the weak comparison back in the login path for every account that has not moved
  yet. Migrating an existing table is a password
  reset per row.

Next: [Sessions and identity](35-sessions-and-identity.md).
