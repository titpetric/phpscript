# Building an application

This walkthrough builds a complete phpscript application from an empty
directory: a bookmark list with a database, migrations, routed endpoints,
compiled templates and an integration test suite.

The finished application is [demos/example](../../demos/example). Every
snippet below is taken from it, and its
[venom suite](../../demos/example/tests/venom.yml) runs in CI, so the code here
stays true.

## 1. The layout

phpscript serves an application directory. Only `public/` is reachable over
HTTP; everything else is code the server loads but never exposes:

```text
example/
├── config.yml                 # connections and server settings
├── migrate.php                # @startup: applies the schema
├── schema/
│   └── bookmarks.up.sql       # one migration file per table
├── bootstrap.php              # shared setup, included by every endpoint
├── index.php                  # @route GET /
├── bookmark-create.php        # @route POST /bookmarks
├── bookmark-delete.php        # @route POST /bookmarks/{id}/delete
├── composer.json              # requires titpetric/minitpl
├── vendor/                    # composer install; not committed
├── templates/
│   └── list.tpl               # compiled to templates/cache/ on first use
└── public/
    └── assets/
        └── style.css          # served directly
```

There is no front controller. Each endpoint is a file, and the annotation at
the top of it is the routing table.

## 2. Configure a connection

`config.yml` names the databases the application uses and enables route
scanning:

```yaml
routes:
  enabled: true

env:
  - "PLATFORM_DB_BOOKMARKS=sqlite://bookmarks.db"
```

The part after `PLATFORM_DB_` is the connection name PHP asks for, lowercased:
this one is `"bookmarks"`. Pass the file with `-f`; without it, phpscript uses
its embedded defaults. See [Configuration](../configuration.md).

## 3. Create the schema

Migrations are `*.up.sql` files, one per table, holding the table definition
and any rows the application needs to start with:

```sql
CREATE TABLE bookmarks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	url TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bookmarks (title, url) VALUES ('phpscript', 'https://github.com/titpetric/phpscript');
```

They are applied by a file carrying the `@startup` annotation, which the server
runs to completion before it listens:

```php
<?php

// @startup

$migrate = new Database\Migrate("bookmarks");

$migrate->load("./schema/*.up.sql");
$migrate->run();
```

Migration files are append only. [Run migrations](database.md#run-migrations)
covers what that means for later schema changes.

## 4. Share setup between endpoints

Every endpoint needs the same database handle and template engine, so they live
in one file that each endpoint includes. `bootstrap.php` is not routed and is
outside `public/`, so it is never reachable on its own:

```php
<?php

include "vendor/autoload.php";

$db = new Database("bookmarks");

$tpl = new MiniTPL\Template("templates/");

$tpl->set_compile_location("cache/", false);

/** Sends a redirect and stops the script. */
function redirect_to($url) {
	header("Location: " . $url);
	exit();
}
```

## 5. Write the endpoints

An endpoint reads its input, talks to the database, and renders or redirects.
The `@route` annotation gives it a method and a path:

```php
<?php

// @route GET /

include "bootstrap.php";
$bookmarks = $db->get_all("SELECT id, title, url, created_at FROM bookmarks ORDER BY id DESC");
$message = "";
if (isset($_GET["saved"])) {
	$message = "Bookmark saved.";
}

$tpl->load("list.tpl");
$tpl->assign(array("title" => "Bookmarks", "bookmarks" => $bookmarks, "message" => $message));
$tpl->render();
$db->close();
```

Writes take a `POST` route, and answer with a redirect so a reload does not
repeat them. `$db->query()` binds its arguments to the `?` placeholders:

```php
<?php

// @route POST /bookmarks

include "bootstrap.php";
$title = trim($_POST["title"]);
$url = trim($_POST["url"]);
if ($title == "" || $url == "") {
	die("A title and a URL are required.");
}

$db->query("INSERT INTO bookmarks (title, url) VALUES (?, ?)", $title, $url);
$db->close();
redirect_to("/?saved=1");
```

Path parameters are declared in braces and arrive in `$_PATH`:

```php
<?php

// @route POST /bookmarks/{id}/delete

include "bootstrap.php";
$id = (int)$_PATH["id"];
$bookmark = $db->get("SELECT id FROM bookmarks WHERE id = ?", $id);
if (!$bookmark) {
	die("Bookmark not found.");
}

$db->query("DELETE FROM bookmarks WHERE id = ?", $id);
$db->close();
redirect_to("/");
```

`die("message")` writes the message and stops the script, which is enough for a
guard clause. [Error handling](error-handling.md) covers exceptions.

Keep all `$_GET` and `$_POST` reads in the annotated files. Shared code that
reaches into superglobals is code whose input you cannot see from the route
that runs it.

## 6. Render the output

Templates are compiled to PHP by the bundled engine, not written as PHP.
`load()` compiles `templates/list.tpl` into `templates/cache/list.tpl` on first
use and recompiles it whenever the source is newer:

```php
$tpl->load("list.tpl");
$tpl->assign(array("title" => "Bookmarks", "bookmarks" => $bookmarks));
$tpl->render();
```

Assigned values are addressed by name in braces, array elements with a dot, and
`|escape` runs the value through `htmlspecialchars`:

```html
<h1>{title|escape}</h1>

{if $message}
<p class="notice">{message|escape}</p>
{/if}

<ul class="bookmarks">
{foreach $bookmarks as $bookmark}
	<li>
		<a href="{bookmark.url|escape}">{bookmark.title|escape}</a>
		<form method="post" action="/bookmarks/{bookmark.id}/delete">
			<button class="link">Delete</button>
		</form>
	</li>
{/foreach}
{if count($bookmarks) == 0}
	<li class="empty">No bookmarks yet.</li>
{/if}
</ul>
```

Escape everything that came from a request. The suite has a case for it: a
bookmark titled `<script>alert(1)</script>` must come back as text.
[Templating](templating.md) documents the rest of the syntax.

## 7. Run it

```bash
phpscript -f config.yml server .
```

The server applies the migrations, scans for annotations and prints the address
it is listening on. Check what it registered:

```bash
phpscript list ./...
```

```text
  | Route                       | Filename                                     | Classes |
  |-----------------------------|----------------------------------------------|---------|
  | POST /bookmarks             | [bookmark-create.php](./bookmark-create.php) | <none>  |
  | POST /bookmarks/{id}/delete | [bookmark-delete.php](./bookmark-delete.php) | <none>  |
  | <none>                      | [bootstrap.php](./bootstrap.php)             | <none>  |
  | GET /                       | [index.php](./index.php)                     | <none>  |
  | @startup                    | [migrate.php](./migrate.php)                 | <none>  |
```

A file with no entry point is only reachable through an `include`. If an
endpoint is missing from this table, its annotation is wrong. `vendor/` is
skipped: a composer dependency does not publish routes into the application.

PHP is parsed when the server boots, so restart it after editing a source file.

## 8. Test it

Endpoints are HTTP, so test them over HTTP. The application's suite is a
[venom](https://github.com/ovh/venom) testsuite:

```yaml
testcases:
  - name: a bookmark can be added
    steps:
      - type: http
        method: POST
        url: "{{.host}}/bookmarks"
        headers:
          Content-Type: application/x-www-form-urlencoded
        body: "title=Venom&url=https://github.com/ovh/venom"
        assertions:
          - result.statuscode ShouldEqual 200
          - result.body ShouldContainSubstring Bookmark saved.
          - result.body ShouldContainSubstring Venom
```

venom follows the redirect, so a `POST` that succeeds is asserted through the
page it lands on.

Write the suite so it can run twice. Rather than assume which row ids exist,
each case that adds a bookmark reads back the id the application gave it and
deletes it again:

```yaml
        extracts:
          result.body: "action=\"/bookmarks/(?P<added>[0-9]+)/delete\""
      - type: http
        method: POST
        url: "{{.host}}/bookmarks/{{.added}}/delete"
```

A named capture group in `extracts` becomes a variable for the steps that
follow, which is how the suite cleans up after itself and stays independent of
what a previous run left behind.

Unit-level behaviour that does not need a server belongs in a `.phpt` fixture
instead; see [Testing](../testing.md).

## Where to go next

- [Routing](routing.md) - annotations, methods and path parameters
- [Database bindings](database.md) - queries, transactions and migrations
- [Templating](templating.md) - the template syntax in full
- [Error handling](error-handling.md) - exceptions from bindings
- [dbadmin](../../demos/dbadmin) - a larger application built the same way
