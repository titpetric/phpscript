# Creating phpscript applications

This is a book about building a complete application on phpscript: one with signed-in and
signed-out users, a private administration surface, a public one, and JSON APIs that are
public where you want them public and authenticated where you do not.

It is written for someone who knows PHP. phpscript runs PHP syntax and a subset of the
standard library, and the subset is opinionated: there is no inheritance, no service
container, no `$_SESSION` and no `curl_*`. Each chapter says what to write instead at the
point you would reach for the missing thing.

Two applications in this repository are the worked examples, and every code block in the
book is taken from one of them:

- [demos/example](../../demos/example) is a bookmark list: one table, three routes, a
  template. It is what [Building an application](../use-cases/application.md) walks through.
- [demos/common-phpscript](../../demos/common-phpscript) is the complete one: users, groups,
  permissions, sessions and CSRF, behind a 22-route administration panel and a five-route
  JSON API, packaged as the composer library `titpetric/phpscript-common`.

## Chapters

1. [Your first application](05-your-first-application.md) - the layout, `config.yml`, a
   first route, running the server.
2. [Routing and endpoints](10-routing-and-endpoints.md) - `@route`, path parameters,
   reading input, `@startup` and `@schedule`.
3. [Databases and migrations](15-databases-and-migrations.md) - connections, `*.up.sql`,
   queries, placeholders, transactions, writing portable SQL.
4. [Templates and rendering](20-templates-and-rendering.md) - minitpl, the search path,
   escaping, HTML and JSON responses.
5. [Structuring an application](25-structuring-an-application.md) - composition instead of
   inheritance, store interfaces, the composition root.
6. [Users and authentication](30-users-and-authentication.md) - the user record, password
   hashing, signing in and out.
7. [Sessions and identity](35-sessions-and-identity.md) - the session row, reading the user
   record, CSRF, flash messages.
8. [Groups and permissions](40-groups-and-permissions.md) - grants, sections, `can()`, and
   declaring what a module manages.
9. [An administration panel](45-an-administration-panel.md) - panels, guards, navigation,
   public and private routes.
10. [A JSON API](50-a-json-api.md) - API routes, public endpoints, authentication, field
    selection, error envelopes.
11. [Testing](55-testing.md) - `.phpt` fixtures, memory stores, the three runners, venom.
12. [Running in production](60-running-in-production.md) - virtual hosts, configuration
    layering, limits, scheduled work.

[Resources](99-resources.md) collects the reference documentation and the analysis behind
the `phpscript-common` package.

## How do I...

The answer to each of these is a few lines. The chapter has the reasoning.

### Add a page

Create `routes/thing.php`, write the annotation, include the prelude, answer.

```php
<?php

// @route GET /thing

include "bootstrap.php";

echo page($html, $frame, "thing.tpl", array("rows" => $db->get_all("SELECT * FROM thing")));
```

There is no front controller and no route table to edit. The annotation is the routing
table, and `phpscript list ./...` prints it. See
[Routing and endpoints](10-routing-and-endpoints.md).

### Add an API endpoint

The same, answering with JSON instead of a template.

```php
<?php

// @route GET /api/thing

include "bootstrap.php";

$json->render(array("data" => $db->get_all("SELECT id, name FROM thing ORDER BY name")));
```

See [A JSON API](50-a-json-api.md).

### Leave an endpoint public

Write no guard. A guard is a call in the file, so an endpoint without one is open to
anybody. Because that is an omission, say so where a reader will look:

```php
<?php

// @route GET /api/status
//
// Public on purpose. It reports that the service is up and nothing about its
// contents, so it needs no session and no permission.

include "bootstrap.php";

$json->render(array("data" => array("status" => "ok")));
```

### Require a signed-in user

One line, first in the file.

```php
require_login($session);          // a page: redirects an anonymous visitor to the login form
api_require_login($session);      // an API: throws Unauthenticated, which answers 401
```

The two differ because an API client has no browser to send to a form. See
[An administration panel](45-an-administration-panel.md) and
[A JSON API](50-a-json-api.md).

### Require a permission

`require_login` proves who the request is from. `can()` decides what they may do.

```php
require_login($session);
require_can($html, $rules["user"], "user.list", array("0"));
```

The third argument is the section list the key is scoped to. `array("0")` is the
module-wide question; a per-record check passes the record's sections, which for the user
panel are the groups it belongs to:

```php
$sections = $groups->group_ids_of($_REQUEST["id"]);
require_can($html, $rules["user"], "user.edit", $sections);
```

See [Groups and permissions](40-groups-and-permissions.md).

### Get the user record from the session

Two calls. The session carries an id; the store carries the record.

```php
$context = $session->current();
$user = $context["is_authenticated"] ? $users->find($context["user_id"]) : false;
```

`current()` always returns an array, so nothing tests it for false. Its keys are
`is_authenticated`, `session_id`, `user_id`, `csrf_token`, `remote_addr` and `user_agent`.
See [Sessions and identity](35-sessions-and-identity.md).

### Sign a user in, and out

```php
$user_id = $auth->attempt($username, $password);   // the id, or false
if ($user_id !== false) {
	$session->start($user_id, $remote_addr, $user_agent);
}

$session->revoke();                                 // sign out
```

`attempt()` does not touch the session, so the credential check is testable without a
request. See [Users and authentication](30-users-and-authentication.md).

### Protect a form against CSRF

The token is on the session row and is already in the frame every template renders.

```php
require_csrf($html, $csrf);        // first line of every POST route
```

```html
<input type="hidden" name="csrf_token" value="{csrf_token}">
```

A signed-out visitor has no session row and therefore no token, which is why the login form
route calls `$session->ensure()` before rendering. See
[Sessions and identity](35-sessions-and-identity.md).

### Query the database

```php
$row  = $db->get("SELECT * FROM user WHERE id = ?", $id);        // one row, or false
$rows = $db->get_all("SELECT * FROM user ORDER BY username");    // an array of rows
$db->query("UPDATE user SET email = ? WHERE id = ?", $email, $id);
```

Always bind. See [Databases and migrations](15-databases-and-migrations.md).

### Add a table

Write `schema/<table>.up.sql`. `migrate.php` carries `@startup`, so the server applies it
before it listens. Migration files are append only. See
[Databases and migrations](15-databases-and-migrations.md).

### Return an error

Throw, and let the route's catch turn it into a response.

```php
throw new Common\NotFound("No such user.");        // 404
throw new Common\ValidationFailed("Name required."); // 422
throw new Common\PermissionDenied("Not yours.");   // 403
throw new Common\Conflict("That name is taken.");  // 409
```

A component throws and a route file exits, because an exit is treated as success and unwinds
past `catch`. See [Templates and rendering](20-templates-and-rendering.md).

### Run something at boot, or on a timer

```php
// @startup                    runs once, to completion, before the server listens
// @schedule hourly            runs for the life of the server
```

See [Routing and endpoints](10-routing-and-endpoints.md).

### Test a component without a database

Every component takes a store interface. Give it the memory implementation.

```php
$users = new Common\Users(new Common\Mock\MemoryUserStore());
```

Then a `.phpt` beside the class exercises it on both engines and on real PHP. See
[Testing](55-testing.md).

### Add an administration panel

Write a class implementing `Common\ModuleInfo` and `Common\AdminPanel`, register it in
`bootstrap.php`, add a route file and a template per screen. The registry is a literal list
because `new $class` is a parse error. See
[An administration panel](45-an-administration-panel.md).

### Serve several applications from one process

The operator's `virtualhost:` list names each site's domain and root, and each site root
carries its own `phpscript.yml`. A site may not set `server` or `virtualhost`. See
[Running in production](60-running-in-production.md).

## Conventions

These hold across the book and across both worked applications.

**Layout.** `public/` is the only directory reachable over HTTP. Route files live in
`routes/`, one per endpoint, named `<prefix>-<action>.php`. Templates mirror them by stem in
`templates/`. Migrations are `schema/<table>.up.sql`, one per table. Scheduled files live in
`jobs/`. `bootstrap.php` and `config.yml` sit at the root.

**Naming.** Classes are `StudlyCaps`, one per file, the file named after the class. Methods
and functions are `snake_case`, matching the `Database` binding and PHP's own library. A
storage interface is `<Thing>Store`, its SQL implementation `Store\Sql<Thing>Store` and its
memory implementation `Mock\Memory<Thing>Store`.

**Schema.** Singular table names. No foreign keys anywhere, so cascade is the application's
job and each component documents what it deletes. Indexes are `idx_` and unique indexes
`uidx_`, both prefixing the table name. Timestamps are native columns defaulting to
`CURRENT_TIMESTAMP`, written and compared by the server, because there is no `date()` here.

**Structure.** Dependencies arrive through `__construct`. There is no container and no
service locator. One responsibility per class: a class and its functions stand in for a Go
struct and its funcs. An interface appears where a seam is needed, and a class that
implements one declares every method itself.

**Guards.** A route file guards itself, first thing, in this order: CSRF, then
authentication, then permission. A route with no guard is public.

**Errors.** A component throws; a route file catches at its own boundary and exits. Every
route ends with the same catch block, so a 404 from a store is a 404 to the client.

**Input.** All `$_GET`, `$_POST` and `$_REQUEST` reads happen in the annotated file, so the
input to a route is visible from the route.

**Tests.** A `.phpt` beside every `.php`. Fixtures run on both engines and on real PHP;
one that touches a host binding opts the PHP runner out and says why.

## What is not there

Worth knowing before you plan around it. Each has a chapter that says what to do instead.

| Missing                                   | Instead                                                  |
|-------------------------------------------|----------------------------------------------------------|
| `extends`, `parent::`, traits, `abstract` | Composition; declare every member a class uses           |
| `new $class`, `$this->$method()`          | A literal registry; `call_user_func(array($obj, $name))` |
| Service container, `__call`               | Constructor injection in `bootstrap.php`                 |
| `$_SESSION`, `session_start`, `setcookie` | `Session\Manager` and a session row                      |
| `md5`, `hash`, `random_bytes`, `rand`     | `password_hash`; tokens minted by the database           |
| `date`, `gmdate`, `strftime`              | Write and compare times in SQL                           |
| `parse_url`, `parse_str`, `intval`        | `Common\Uri`; a `(int)` cast                             |
| `curl_*`                                  | `HTTP\Client`                                            |
| PDO, `mysqli_*`                           | `Database`, `Database\Migrate`                           |
| Array copy on assignment                  | Arrays are handles; copy with a `foreach`                |
| A route with a default path parameter     | Write the path literally, one file per action            |

## Next

[Your first application](05-your-first-application.md).
