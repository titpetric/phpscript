# Routing and endpoints

An endpoint is one PHP file with one comment at the top of it. This chapter adds one, then
covers what the comment may say, how the request reaches the file, and how the file answers.
It ends with the two annotations that run code with no request behind them, `@startup` and
`@schedule`, and with the command that tells you what the server actually registered.

## Add an endpoint

Create a file under `routes/`, one file per endpoint. This is
`routes/admin-users-edit.php`, the whole file:

```php
<?php

// @route GET /admin/user/{id}

use Common\Render\Problem;

include "bootstrap.php";

try {
	require_login($session);

	$user = $users->find($_REQUEST["id"]);
	if ($user === false) {
		fail($html, 404, "No such user.");
	}

	$member_of = $groups->group_ids_of($_REQUEST["id"]);
	require_can($html, $rules["user"], "user.edit", $member_of);

	echo page($html, $frame, "admin-users-edit.tpl", array(
		"user" => $user,
		"member_of" => $groups->groups_of($_REQUEST["id"]),
		"all_groups" => $groups->all(),
		"message" => $flash->take(),
	));
} catch (Exception $e) {
	$problem = Problem::of($e);
	http_response_code(Problem::status($e));
	echo $html->get("_exception.tpl", $problem["error"]);
}
```

Five parts, in order:

1. `// @route GET /admin/user/{id}` publishes the file. The server scans the tree for these
   comments when it starts, so a new file needs a restart before it answers.
2. `include "bootstrap.php"` is the prelude. A top level include shares the includer's
   scope, so `$users`, `$groups`, `$rules`, `$html`, `$frame` and `$session` are this file's
   variables and the route constructs nothing.
   [Structuring an application](25-structuring-an-application.md) covers what is in there.
3. `$_REQUEST["id"]` is the input, read here in the annotated file and nowhere below it.
4. `echo` is the answer. Everything the script writes is the response body.
5. The `catch` turns an exception thrown by a component into a status and an error page.
   Guards inside the body answer directly with `fail()`.

The file name is a convention rather than a rule the runtime enforces: `routes/<stem>.php`
renders `templates/<stem>.tpl`, so the two directories sort into the same order.

## Declare methods and paths

The annotation is `// @route [METHOD] /path/{param}`. It is repeatable, one per line, and a
file may carry as many as it needs:

```php
// @route PUT /report/{id}
// @route DELETE /report/{id}
// @route HEAD /report/{id}
// @route OPTIONS /report/{id}
```

A path with no method registers both GET and POST. `annotations/parse.go` expands a one
field annotation into those two, so `// @route /admin/login` answers the form and the
submission from one file. The application splits them into `admin-auth-form.php` and
`admin-auth-login.php` instead, because a GET that renders and a POST that writes share
nothing but the URL.

Every other method is written out, PUT, DELETE and PATCH as expected, and HEAD and OPTIONS
as well: a GET route does not answer HEAD, and nothing answers OPTIONS, so both are 404
until a file declares them.

Two files declaring the same method and path both register, the one later in path order
wins, and the server logs `WARN duplicate route ...` while it starts.

## Take path parameters out of $_REQUEST

`{name}` in the path matches one segment, and the segment arrives in `$_REQUEST` under that
name as a string:

```php
// @route POST /admin/user/{id}/group/{group_id}/delete

$id = $_REQUEST["id"];
$group_id = $_REQUEST["group_id"];
```

Three spellings route and reach the script — `model.ParseRoutePath` is the one grammar an
annotation is written against, and every parameter it accepts arrives under the name it
declares: `{name}` for one segment, `{name...}` for the remaining segments joined, and
`{name:regex}` for one constrained segment. Anything else is refused: the route is not
registered, the server logs it at boot with the file it came from, and `phpscript lint`
reports it with a line number.

| Annotation          | Request           | Result                                                                          |
|---------------------|-------------------|---------------------------------------------------------------------------------|
| `/admin/user/{id}`  | `/admin/user/abc` | serves, `$_REQUEST["id"]` is `"abc"`                                            |
| `/admin/user/{id}`  | `/admin/user/a/b` | 404, a parameter is one segment                                                 |
| `/g/{rest...}`      | `/g/a/b`          | serves, `$_REQUEST["rest"]` is `"a/b"`                                          |
| `/x/{id:[0-9]+}`    | `/x/12`           | serves, `$_REQUEST["id"]` is `"12"`                                             |
| `/x/{id:[0-9]+}`    | `/x/ab`           | 404 under the bundled server; an `http.ServeMux` host serves, unconstrained     |
| `/q/{id:.*}`        | `/q/a/b`          | 404, a constrained parameter is still one segment; `{rest...}` is the catch-all |
| `/y/{module=users}` | `/y/anything`     | refused at boot, there is no default-value syntax                               |
| `/z/{module}/*`     | `/z/users/a/b`    | serves under chi with `module` and the tail nowhere; write `{rest...}` instead  |

`{name...}` is the catch-all, and the one spelling that matches across segments: a regex
constraint applies within a single segment, so `{id:.*}` is not a tail. The constraint is
enforced by the bundled server, which routes with chi; a host registering on
`http.ServeMux` gets the parameter without it, because `ServeMux` has no equivalent — see
[Routing](../use-cases/routing.md). There is no default-value syntax:
`/admin/{module=users}/{path}` is an authoring error rather than a route that defaults
`module`. Values are strings, and `intval` is not bound, so cast with
`(int)$_REQUEST["id"]` when you need a number.

## Read the request

Six superglobals carry the request: `$_GET`, `$_POST`, `$_REQUEST`, `$_COOKIE`, `$_SERVER` and
`$_FILES`. Every value is a string, and a key that was not sent reads as `null` with no
notice, so test with `isset()` wherever absence and an empty value differ:

```php
$name = isset($_POST["name"]) ? trim($_POST["name"]) : "";
```

`$_GET` and `$_POST` are flat maps of one value per key. A key sent twice keeps the last
value. There is no `name[a][b]` decoding: a field named `user[name]` arrives as the literal
key `user[name]`, and nothing splits it for you.

Name fields so they can be rebuilt rather than parsed. The permission grid does this. Its
form names every checkbox `rule.<role>.<permission>.<section>`, and
`routes/admin-permissions-save.php` walks the module declaration to reconstruct those names
instead of reading the keys that arrived:

```php
foreach ($role["role"] as $permission) {
	foreach ($sections as $section_id) {
		$field = "rule." . $role["name"] . "." . $permission["name"] . "." . $section_id;
		$value = isset($_POST[$field]) ? $_POST[$field] : "inherit";

		$rule_store->set($module["id"], $group["id"], $role["name"], $permission["name"], $section_id, $value);
	}
}
```

Walking the declaration also decides what a missing field means, which is inherit here, and
stops a caller from writing a grant for a key the module never declared.
[Groups and permissions](40-groups-and-permissions.md) covers the declaration itself.

`$_SERVER` holds what the request itself answers for: `REQUEST_METHOD`, `REQUEST_URI`,
`QUERY_STRING`, `HTTP_HOST`, `SERVER_PROTOCOL`, `REMOTE_ADDR`, `REMOTE_PORT`,
`REQUEST_SCHEME`, `CONTENT_TYPE`, `CONTENT_LENGTH`, `REQUEST_TIME`, and one `HTTP_*` key per
header. The keys a SAPI fills by resolving a URL to a file on disk, `DOCUMENT_ROOT`,
`SCRIPT_NAME`, `PHP_SELF`, `SERVER_NAME` and `SERVER_PORT`, are not there. `$_FILES` is
keyed by field name, each entry carrying `name`, `full_path`, `type`, `tmp_name`, `error`
and `size`, and a field named `files[]` takes PHP's parallel array shape.

Keep all of these reads in the annotated file, which is the rule
[`docs/use-cases/application.md`](../use-cases/application.md) states. Shared code that
reaches into a superglobal is code whose input you cannot see from the route that runs it.

## Answer the request

`echo` writes the body. `header()` stages a response header, `http_response_code()` stages a
status, and `redirect_to()` from `bootstrap.php` does both and stops:

```php
function redirect_to($url)
{
	header("Location: " . $url);
	exit();
}
```

A `Location` header defaults the status to 302, the same as PHP. Every route that writes
ends in a redirect, so a reload asks for a page instead of repeating the write. This is
`routes/admin-users-save.php`, complete:

```php
<?php

// @route POST /admin/user/{id}

use Common\Render\Problem;

include "bootstrap.php";

try {
	require_login($session);
	require_csrf($html, $csrf);

	$user = $users->find($_REQUEST["id"]);
	if ($user === false) {
		fail($html, 404, "No such user.");
	}

	$member_of = $groups->group_ids_of($_REQUEST["id"]);
	require_can($html, $rules["user"], "user.edit", $member_of);

	$username = isset($_POST["username"]) ? trim($_POST["username"]) : "";
	if ($username === "") {
		fail($html, 422, "A username is required.");
	}

	$users->update($_REQUEST["id"], array(
		"username" => $username,
		"email" => isset($_POST["email"]) ? trim($_POST["email"]) : "",
		"is_admin" => isset($_POST["is_admin"]) ? 1 : 0,
		"is_active" => isset($_POST["is_active"]) ? 1 : 0,
	));

	$flash->set("Saved " . $username . ".");

	redirect_to("/admin/user/" . $_REQUEST["id"]);
} catch (Exception $e) {
	$problem = Problem::of($e);
	http_response_code(Problem::status($e));
	echo $html->get("_exception.tpl", $problem["error"]);
}
```

The order is fixed and every POST route in the application repeats it: authenticate, check
the CSRF token, load the row, check the permission against that row, validate the input,
write, set the flash message, redirect. The message survives the redirect in the session and
the GET route displays it. [Sessions and identity](35-sessions-and-identity.md) covers the
flash and the token. A POST route has no template of its own, because it never renders one.

## exit and die

`exit` and `die` end the script. The staged status stands, the body written so far is kept,
and an exit code of 0 is an ending rather than a failure. On the command line the exit code
you pass is the one `phpscript run` exits with.

An exit is not an exception. It unwinds past `catch` and skips `finally`:

```php
try {
	echo "before\n";
	exit();
} catch (Exception $e) {
	echo "caught\n";
} finally {
	echo "finally\n";
}
echo "after\n";
```

That prints `before` and nothing else, and answers 200.

So a route file may exit, and `fail()` and `redirect_to()` do exactly that: guard clauses
that end a request in one line, declared in `bootstrap.php` where the exit is visible. A
component throws instead. A class that exits cannot be wrapped, retried or tested, because
the caller's `catch` never runs and its `finally` never releases anything. This rule runs
through the whole book: route files exit, everything below them throws.

## Run work before the server listens

`// @startup` marks a file the server runs to completion before it accepts a request. The
migration file is the example:

```php
<?php

// @startup

$migrate = new Database\Migrate("common");
$migrate->load("./schema/*.up.sql");
$migrate->run();

echo "migrate: schema applied\n";

include "bootstrap.php";
```

The migration runs before `bootstrap.php` is included, not after: the composition root
queries tables while it builds the navigation, and on a fresh database those tables do not
exist yet. A request never meets a half migrated schema, and a startup file that fails stops
the server from coming up. [Databases and migrations](15-databases-and-migrations.md) covers
`Database\Migrate`.

## Run work on a schedule

`// @schedule <spec>` runs a file for the life of the server. `jobs/session-prune.php` is
one file, one annotation, and the same prelude every route uses:

```php
<?php

// @schedule hourly

include "bootstrap.php";

echo "session-prune: removed " . $session->prune() . " sessions\n";
```

`annotations/schedule_parse.go` accepts exactly these specs:

| Spec                 | Runs                                              |
|----------------------|---------------------------------------------------|
| `hourly`             | every hour                                        |
| `daily`              | at midnight                                       |
| `weekly`             | Sunday at midnight                                |
| `monthly`            | on the first of the month at midnight             |
| `every weekday`      | Monday to Friday at midnight                      |
| `every <day>`        | that weekday at midnight, named `monday` or `mon` |
| `every <n> <unit>`   | every n `seconds`, `minutes` or `hours`           |
| `<n> times per hour` | n times an hour, evenly spaced                    |
| `<n> times per day`  | n times a day, evenly spaced                      |

Anything else is dropped without an error, so `@daily`, a cron expression and
`every 10 days` register no job at all. An interval spec counts from when the server
started, not from the top of the hour. A day aligned spec fires at local midnight.

Arguments after `--` become `$argv`, with the file name as `$argv[0]`:

```php
// @schedule every 5 minutes -- report --verbose
```

A tick is skipped while the previous run is still going: the scheduler holds a lock per job
and does not queue. The lock is one mutex in one process, so two servers running the same
tree each run the job on their own clock.

## Check what the server registered

`phpscript list ./...` prints every route, startup file and scheduled job in the tree as a
markdown table, sorted by file name. Six rows of it, from an application with 22 routes:

```text
  | Route                 | Filename                                                     | Classes |
  |-----------------------|--------------------------------------------------------------|---------|
  | @schedule hourly      | [jobs/session-prune.php](./jobs/session-prune.php)           | <none>  |
  | @startup              | [migrate.php](./migrate.php)                                 | <none>  |
  | GET /admin/login      | [routes/admin-auth-form.php](./routes/admin-auth-form.php)   | <none>  |
  | POST /admin/login     | [routes/admin-auth-login.php](./routes/admin-auth-login.php) | <none>  |
  | GET /admin/user/{id}  | [routes/admin-users-edit.php](./routes/admin-users-edit.php) | <none>  |
  | POST /admin/user/{id} | [routes/admin-users-save.php](./routes/admin-users-save.php) | <none>  |
```

A file you expected and cannot find is a file with a typo in its annotation, and a schedule
spec that did not parse is absent for the same reason.

Both the scanner and this command skip `vendor/`. A composer package therefore cannot
publish routes into an application: a file carrying `// @route` under
`vendor/titpetric/phpscript-common/routes/` is never registered and never served. A package
that ships routes ships them as files to copy, and the application copies them once:

```bash
cp -n vendor/titpetric/phpscript-common/routes/*.php routes/
```

`-n` never overwrites, so a route the application edited survives an upgrade, and the list
is the check that the copy took.

The route scan skips two further directories, and only the route scan does. Files under the
document root are served directly, so an annotation there would publish a second, unguarded
route. A script can write a `.php` file into one of `runner.writable_paths`, so scanning
those would let an upload register a route at the next restart.

Next: [Databases and migrations](15-databases-and-migrations.md).
