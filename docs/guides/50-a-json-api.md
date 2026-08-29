# A JSON API

This chapter adds `/api` to the application built in [An administration panel](45-an-administration-panel.md). The API routes include the same `bootstrap.php`, use the same components and answer the same exceptions as the admin pages; what changes is that they render through `Common\Render\Json` and that their guard throws instead of redirecting. At the end you have five endpoints: four behind a session and one left open on purpose.

## Adding an API endpoint

Four steps.

1. Create `routes/api-<panel>-<action>.php` and put a `// @route` line at the top. The stem is flat and hyphenated, the same convention the admin routes follow.
2. `include "bootstrap.php"`. The connection, the stores, the components, `$json` and `$fields` are all built there, and a top level include shares the includer's scope, so the route file constructs nothing.
3. Guard it. `api_require_login($session)` decides who is calling, `$rules["user"]->can($key, $sections)` decides what they may do.
4. Answer inside a `try`, with `$json->render(array("data" => ...))`, and answer the `catch` with `Common\Render\Problem`.

The complete file:

```php
<?php

// @route GET /api/user

use Common\Render\Problem;

include "bootstrap.php";

try {
	api_require_login($session);

	if (!$rules["user"]->can("user.list", array("0"))) {
		throw new Common\PermissionDenied("you may not user.list");
	}

	// The row is projected column by column. A stored user row carries the
	// bcrypt hash in "password" and json_encode() would publish it, and
	// created_at is a DATETIME that arrives as a Go time object.
	$rows = array();
	foreach ($users->all() as $row) {
		$rows[] = array(
			"id" => $row["id"],
			"username" => $row["username"],
			"email" => $row["email"],
			"is_admin" => (int)$row["is_admin"],
			"is_active" => (int)$row["is_active"],
		);
	}

	$json->render(array("data" => $rows));
} catch (Throwable $e) {
	http_response_code(Problem::status($e));
	$json->render(Problem::of($e));
}
```

The route has to catch. Letting an exception escape does not produce a status: the runtime picks the status of a failed request by calling `GetCode()` on the error the script ended with, and the wrapper it puts around a thrown object does not implement it, so an uncaught `Common\NotFound` ends the request as a 500 whatever its code property holds.

`phpscript list ./...` is the check that the file registered, here with the classes column cut:

```text
| GET /api/status       | routes/api-status.php        |
| POST /api/user        | routes/api-users-create.php  |
| DELETE /api/user/{id} | routes/api-users-delete.php  |
| GET /api/user         | routes/api-users-list.php    |
| GET /api/user/{id}    | routes/api-users-read.php    |
```

## Leaving an endpoint public

A guard is a call in the file. Nothing outside the file grants access, no annotation withholds it and the routing table has no column for it, so an endpoint that calls neither `api_require_login()` nor `can()` is open to anyone who can reach the port. Public is a decision made by omission, and an omission looks the same whether it was meant or not.

Write it down. `routes/api-status.php` says what it does and what it withholds:

```php
<?php

// @route GET /api/status

/**
 * /api/status is public on purpose.
 *
 * There is no guard in this file, and that is the decision rather than an
 * omission. A guard in this application is a call in the route file:
 * require_login() under /admin, api_require_login() under /api. An endpoint
 * that calls neither is open to anyone who can reach the port, and nothing in
 * the annotation, the file name or the routing table says so.
 *
 * What this endpoint answers: the site name, that the process is up, whether
 * the request carried a session this server still recognises, and the username
 * on that session.
 *
 * What it does not answer: no user list, no user count, no group or permission
 * names, no session id, no CSRF token, no database or build string, and
 * nothing at all about a caller who sent no credential. Every anonymous
 * request gets the same body as every other anonymous request, which is the
 * test to apply to an endpoint before leaving it open.
 */
```

Two claims make the comment checkable by someone who did not write it: the list of what the response carries, and the sentence that every anonymous request gets the same body. A reviewer can run the endpoint and compare.

```bash
curl -s http://127.0.0.1:8080/api/status
```

```text
{"data":{"status":"ok","site":"Common","authenticated":false,"username":""}}
```

## Adding authentication to an endpoint

One line, before anything else in the `try`:

```php
	api_require_login($session);
```

It throws `Common\Unauthenticated`, whose default code is 401, and the catch turns that into the status and the body. The admin twin, `require_login($session)` in `bootstrap.php`, sends a 302 to `/admin/login` with the original URI in `next`. A script following that redirect gets a 200 carrying a login page, which is the wrong answer to give a client that cannot fill one in.

It lives in `bootstrap.php`, beside `require_login()`, because every route file already includes that and a guard copied into five files is a guard that can be edited in four. Like the other prelude functions it takes `$session` as an argument: a function body does not see the includer's scope, and `global` parses while doing nothing.

`bootstrap.php` belongs to the application rather than to the package, so adding an endpoint of your own means editing a file you own. Every file under `routes/` is an endpoint, which is what makes the install step a copy of endpoints:

```bash
cp -n vendor/titpetric/phpscript-common/routes/*.php routes/
```

A helper file in the same directory would be a file that has to be copied for another file to work, and one `phpscript list` shows with no route against it. The function is six lines. An application that owns `bootstrap.php` can move it there and drop the copies.

Both client shapes resolve through the same call. `Session\Manager::currentID` in `stdlib/session/manager.go` reads an `Authorize` request header first and the session cookie second, so a browser and a token client reach the same `user_session` row through `Common\Session::current()`. The header carries the session id and nothing else: `validSessionID` requires 64 lowercase hexadecimal characters, so `Authorize: Bearer <id>` is rejected and the request is anonymous.

```bash
curl -s -i http://127.0.0.1:8080/api/user | head -1
curl -s -b cookies.txt http://127.0.0.1:8080/api/user
curl -s -H "Authorize: $SID" http://127.0.0.1:8080/api/user
```

```text
HTTP/1.1 401 Unauthorized
{"data":[{"id":"1e2fd564dc13dfa7a6aac2f50b","username":"admin","email":"","is_admin":1,"is_active":1}]}
{"data":[{"id":"1e2fd564dc13dfa7a6aac2f50b","username":"admin","email":"","is_admin":1,"is_active":1}]}
```

`$SID` is the value of the `session` cookie the sign-in at `POST /admin/login` set. There is no separate token table: a token is a session.

CSRF is not checked on the API writes, and `routes/api-users-create.php` says so where `require_csrf()` would have gone. A CSRF token defends a cookie, because a browser attaches one to a cross-site request without being asked, and a form post therefore needs a second value the attacking page cannot read. A client authenticating with an `Authorize` header attaches nothing by itself. The cookie `Session\Manager` writes is `SameSite=Lax`, which a cross-site POST does not carry, and the browser surface of this application is `/admin`, where every write does check the token.

## Authorising, not just authenticating

`api_require_login()` proves who the caller is. It says nothing about what they may do, and every endpoint after it asks `Common\Rules`.

The list endpoint asks the module-wide question:

```php
	if (!$rules["user"]->can("user.list", array("0"))) {
		throw new Common\PermissionDenied("you may not user.list");
	}
```

The read endpoint asks a scoped one. Sections for the `user` panel are group ids, so the grant answers "may this caller read users in group X":

```php
	$member_of = $groups->group_ids_of($_REQUEST["id"]);
	if (!$rules["user"]->can("user.read", $member_of)) {
		throw new Common\PermissionDenied("you may not user.read");
	}
```

`user.list` and `user.read` are both dotted keys, and `Common\Rules` refuses to default the section of a dotted key: a caller that forgot the section is asking a different question from the one it thinks it is asking. `array("0")` is the module-wide section written out, which is the honest answer for a listing. `can()` appends `"0"` to any section list it is given, so a target in no group falls back to the module-wide grant.

Granting `user.read` to the group `editors` at the section `editors`, and nowhere else, gives a member of that group this:

```text
GET /api/user/9ed5469b60565fc97b8608f020   200   (the target is in editors)
GET /api/user/1e2fd564dc13dfa7a6aac2f50b   403   {"error":{"message":"you may not user.read","code":403}}
GET /api/user                              403   {"error":{"message":"you may not user.list","code":403}}
```

`require_can()` from `bootstrap.php` is not used in an API route. It renders the HTML error page through `fail()` and exits, which would answer a JSON client with a page. Throwing keeps the catch at the foot of the file as the one place a condition becomes a response. [Groups and permissions](40-groups-and-permissions.md) covers the grant model itself.

## Getting the signed-in user's record

Two calls, the same two as in [Sessions and identity](35-sessions-and-identity.md), and they work unchanged in an API route. This is from `routes/api-status.php`:

```php
	$context = $session->current();
	$user = $context["is_authenticated"] ? $users->find($context["user_id"]) : false;
```

`current()` always returns an array, so the test is the `is_authenticated` key and never a comparison against false. The context carries `is_authenticated`, `session_id`, `user_id`, `csrf_token`, `remote_addr` and `user_agent`, and no user columns at all, so the record is a second call.

An `/api/me` endpoint is those two lines followed by `$json->render(array("data" => $record))` over the columns the caller may read. Do not render `$user` whole: the row carries the bcrypt hash in `password`. `/api/status` answers the same question for a caller that may not be signed in, which is why it reports the username as the empty string rather than 401.

## Shaping the response

`Common\Render\Json::render()` sets `Content-Type: application/json` and echoes the body. It echoes rather than exiting, so a route can send a trailer or close a span after it. `json_encode()` takes one argument in this runtime, so there is no pretty-printed variant, and `json_encode(array())` is `[]`, so `Json::get()` writes `{}` for an empty payload instead.

Every successful body is `{"data": ...}` and every error body is `{"error": {"message": ..., "code": ...}}`, so a client reads one key or the other and never has to guess which shape arrived.

Field selection is `Common\Fields`, driven by `?fields=`:

```php
	$selection = isset($_GET["fields"]) ? $_GET["fields"] : "";
	if ($selection !== "") {
		try {
			$record = $fields->apply($selection, $record);
		} catch (Exception $bad) {
			throw new Common\ValidationFailed($bad->getMessage());
		}
	}
```

```bash
curl -s -H "Authorize: $SID" --get \
  --data-urlencode 'fields={ id username }' http://127.0.0.1:8080/api/user/$ID
curl -s -H "Authorize: $SID" --get \
  --data-urlencode 'fields={ username groups { name } }' http://127.0.0.1:8080/api/user/$ID
```

```text
{"data":{"id":"9ed5469b60565fc97b8608f020","username":"alice"}}
{"data":{"username":"alice","groups":[{"name":"editors"}]}}
```

`Fields` filters an array the route already assembled down to the shape the caller named. It is not GraphQL, and calling it that sets a reader up to be wrong: there are no arguments, aliases, fragments, variables, directives, operations, types or resolvers, and the parser never asks what a field means. A name the data does not carry is absent rather than fetched, and naming fewer fields does not make the query cheaper. The route rebuilds the group membership as a list before filtering it, because `Groups::groups_of()` returns rows keyed by group id, which `json_encode()` writes as an object and which `Fields` reads as one record rather than as a list of them.

A malformed selection raises a plain `\Exception` out of the parser, and a plain exception carries no code, which `Problem::status()` reads as 500. The selection is the caller's input, so the route converts it:

```text
{"error":{"message":"fields: unbalanced braces in selection","code":422}}
```

Writes take a form body, not JSON. There is no `php://input` and no raw body binding in this runtime, so only an urlencoded or multipart body reaches `$_POST`; a request sent as `application/json` arrives with `$_POST` empty and every field looking absent. `POST /api/user` reads `$_POST["username"]` and answers 201 with the created record.

## Errors

`Common\Errors` declares six classes. The code each constructor defaults to is the status the response carries.

| Class                     | Status | Raised when                                      |
|---------------------------|--------|--------------------------------------------------|
| `Common\NotFound`         | 404    | no record answers to an identifier               |
| `Common\ValidationFailed` | 422    | input is rejected before any work is done        |
| `Common\Unauthenticated`  | 401    | there is no session, or it expired               |
| `Common\PermissionDenied` | 403    | the rules answered no for a known caller         |
| `Common\Conflict`         | 409    | the write cannot be applied to the current state |
| `Common\ConfigError`      | 500    | the application is wired wrong                   |

`Problem::status($e)` returns the exception's code when it is between 400 and 599 and 500 otherwise, which is the same rule the runtime applies to an uncaught error. `Problem::of($e)` builds `array("error" => array("message", "code"))`.

At 500 and above the message is replaced with the fixed string `internal error`, so the body is `{"error":{"message":"internal error","code":500}}` whatever failed. A status of 500 means the caller cannot act on it, and the text of an internal failure names table columns, file paths and settings.

Which `catch` clause takes a class is decided by the tail of its fully qualified name. A name ending in `Error` is taken by `catch (Error $e)`, any other name by `catch (Exception $e)`, and every name by `catch (Throwable $e)`. That is why the five conditions a caller is expected to handle avoid the suffix and `ConfigError` carries it. The API routes catch `Throwable`, so a `ConfigError` is answered 500 with its message withheld; a route catching `Exception` alone would let it escape. The admin routes hand the same `Problem::of($e)` payload to `Common\Render\Html` instead, so a 404 is a 404 on both surfaces and neither renderer knows an exception exists.

## Methods

`// @route DELETE /api/user/{id}` registers the method as written, and `$_REQUEST` is filled from the matched pattern whatever the method is, so `{id}` arrives in a `DELETE` handler exactly as it does in the `GET` twin. `PUT` behaves the same way. The admin route is `POST /admin/user/{id}/delete` only because an HTML form emits GET and POST and nothing else.

| Annotation                        | Registers    |
|-----------------------------------|--------------|
| `// @route GET /api/user`         | GET only     |
| `// @route DELETE /api/user/{id}` | DELETE only  |
| `// @route: /api/user`            | GET and POST |
| `// @route HEAD /api/status`      | HEAD only    |

A path-only annotation expands to GET and POST, and to nothing else. HEAD and OPTIONS are not implied by a GET route and have to be written out. Without them the server answers 404, not 405, and the same goes for a method no annotation registered: `HEAD /api/status`, `OPTIONS /api/status` and `POST /api/user/{id}` are all 404 against this application.

A path takes `{name}`, `{name...}` and `{name:regex}` parameters, each arriving in `$_REQUEST` under its declared name; there is no default-value syntax. See [Routing and endpoints](10-routing-and-endpoints.md).

## What this API does not do

There is no rate limit. Every endpoint accepts as many requests as the client sends, and `POST /api/user` will keep creating users until the disk fills. The annotation proposals in `../../demos/common-report/proposal-annotation-ratelimit.md` and `proposal-annotation-limits.md` describe what a declarative limit would look like.

There is no response cache. Each request re-reads the session row, touches it, and re-runs the permission query. `../../demos/common-report/proposal-annotation-cache.md` covers the shape a cached endpoint would take.

There is no pagination on `GET /api/user`. `Users::all()` returns every row in one array and the route encodes all of it, so the response grows with the table. Adding a limit and an offset means a new store method, not a change to the route file alone.

Next: [Testing](55-testing.md) covers the `.phpt` fixtures, the memory stores and driving these endpoints from venom.
