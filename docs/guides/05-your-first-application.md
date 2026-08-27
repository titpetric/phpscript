# Your first application

This chapter builds a directory that serves a page. You write a configuration file, a route
file and a shared prelude, start the server, and read the routing table back out of the
tree. At the end of it you have a working application root and you know which file the
server runs for a given URL.

## What phpscript serves

phpscript serves an application directory. You point `phpscript server` at the root of it
and the whole tree is code the server loads; only `public/` is reachable over HTTP. A file
below `public/` is served as it stands, and a `.php` file there is executed as an
entrypoint. Everything above it is loaded by the server and never exposed, so a credential
in `bootstrap.php` or a query in `src/` is not something a visitor can fetch.

There is no front controller. Each endpoint is a file, and the annotation at the top of it
is the routing table:

```php
<?php

// @route GET /
```

The server scans the tree for those annotations when it starts and registers what it finds.
Nothing dispatches on a URL at request time and no rewrite rules are involved. To find out
what answers `/admin/users`, search for the annotation.

Two directories are skipped by the scan: `public/`, because a file there is already served
directly and a second unguarded route to it would be a defect, and `vendor/`, because a
composer package must not be able to publish routes into an application that depends on it.

## The layout

```text
hello/
├── config.yml                 # connections, document root, writable paths
├── bootstrap.php              # the shared prelude, included by every entrypoint
├── index.php                  # @route GET /
├── migrate.php                # @startup: applies the schema      (optional here)
├── schema/                    # one *.up.sql file per table       (optional here)
│   └── bookmarks.up.sql
├── routes/                    # one file per endpoint             (optional here)
├── templates/                 # .tpl sources                      (optional here)
│   ├── hello.tpl
│   └── cache/                 # compiled templates, written at runtime
├── composer.json              # requires titpetric/minitpl        (optional here)
├── vendor/                    # composer install, not committed   (optional here)
└── public/                    # the only directory reachable over HTTP
    └── assets/
        └── style.css
```

Only `config.yml` and one annotated PHP file are needed to serve a request. The entries
marked optional arrive in later chapters: `schema/` and `migrate.php` in
[Databases and migrations](15-databases-and-migrations.md), `templates/` in
[Templates and rendering](20-templates-and-rendering.md), `composer.json` and `vendor/` with
the template engine. `public/` may be absent while nothing static is served; the server
starts and routes without it.

Route files may sit at the root or in a subdirectory. The scanner walks the whole tree, so
`index.php` at the root and `routes/admin-users-list.php` are registered the same way.
`demos/example` keeps them at the root; `demos/common-phpscript` puts its twenty-three in
`routes/`.

## Configure the application

Create `config.yml`. This is `demos/common-phpscript/config.yml`, and it names everything an
application of this shape needs:

```yaml
routes:
  enabled: true

document_root: public

runner:
  writable_paths:
    - templates/cache
    - common.db

env:
  - "PLATFORM_DB_COMMON=sqlite://common.db"
```

`routes.enabled` turns on the annotation scan. With it off, only files under `public/` are
reachable and no `@route` anywhere else is registered.

`document_root` is the directory served over HTTP, relative to the application root. It is
`public`, and the key exists for a tree that already calls that directory something else.

`runner.writable_paths` lists the directories and files a script may write to, relative to
the application root. A write outside the list throws rather than returning `false`, so a
script cannot carry on as though it had stored something. An empty list, the default, allows
every write. Naming the paths also changes what the server does with them: a `.php` file in
a writable directory is served as bytes instead of executed, and an annotation in one is not
scanned, so an uploaded file cannot publish a route.

`env` registers named database connections. Each entry is
`PLATFORM_DB_<NAME>=<driver>://<dsn>`. The part after the prefix, lowercased, is the name PHP
asks for:

```php
$db = new Database("common");
```

`PLATFORM_DB_COMMON` gives `"common"`, `PLATFORM_DB_REPORTING` gives `"reporting"`. The list
goes to the connection registry and nowhere else. It is not put into the process environment
and not into any PHP variable, so a script cannot read the DSN back:

```php
var_dump(getenv("PLATFORM_DB_COMMON"));        // bool(false)
var_dump(isset($_ENV["PLATFORM_DB_COMMON"]));  // bool(false)
```

The credential is reachable through `new Database(...)` and by no other route.
[Databases and migrations](15-databases-and-migrations.md) uses the connection. Pass the
file with `-f`; without it phpscript uses the defaults compiled into the binary and does not
search the working directory for a `config.yml`.

## Write the first route

Create `index.php`:

```php
<?php

// @route GET /

echo "Hello from phpscript.\n";
```

That is the whole file. The annotation is a comment in the file's header, before any
statement, and it carries a method and a path. What the script writes is the response body.

Start the server from the application root:

```bash
phpscript -f config.yml server .
```

```text
2026/08/26 16:30:04 [platform] started 5 modules: [telemetry phpstartup phpschedule phproute phpserver]
2026/08/26 16:30:04 Server listening on [::]:8080 http://127.0.0.1:8080
2026/08/26 16:30:04 [router] registered 21 routes and 21 middlewares
2026/08/26 16:30:04 GET / -> github.com/titpetric/phpscript/annotations.(*handler).file.1
```

The listing continues with the debug front end and the catch-all that serves `public/`. The
line to look for is the one naming your route. Fetch it:

```bash
curl -s -i http://127.0.0.1:8080/
```

```text
HTTP/1.1 200 OK
Request-Id: 01M0Z7MWKEV3KMGXPT552RS4QW
Date: Wed, 26 Aug 2026 14:30:07 GMT
Content-Length: 22
Content-Type: text/plain; charset=utf-8

Hello from phpscript.
```

The status is 200 because nothing set another one, and the content type is guessed from the
body. A route that writes HTML gets `text/html`; a route that answers JSON sets the header
itself. A file under `public/` is served without any of this: `public/assets/style.css`
answers at `/assets/style.css`, with the path relative to the document root.

## Read the routing table back

`phpscript list` walks the same tree the server scans and prints what it found. Run it from
the application root:

```bash
phpscript list ./...
```

```text
  | Route      | Filename                         | Classes |
  |------------|----------------------------------|---------|
  | <none>     | [bootstrap.php](./bootstrap.php) | <none>  |
  | GET /      | [index.php](./index.php)         | <none>  |
  | GET /scope | [scope.php](./scope.php)         | <none>  |
```

A row with `<none>` in the Route column is a file with no entry point. It is only reachable
through an `include` from a file that does have one. `bootstrap.php` is that here, and being
unroutable is what makes it safe to build a database handle in.

A route you wrote and cannot find in the table has a wrong annotation. The scanner takes
`//`, `#` and `/* ... */` comments, so the usual cause is a missing `@`: a file whose header
reads `// route GET /broken` lists with `<none>` and answers 404. Check the table before
checking the server.

`vendor/` is absent from the listing even when it holds annotated files, which is why an
application using a package that ships route files copies them into its own `routes/`; see
`demos/common-phpscript/README.md`.

## Share setup between routes

Every route needs the same database handle, the same template engine and the same helpers.
They go in one file that each route includes. Create `bootstrap.php`:

```php
<?php

$site_name = "Hello";

$greeting = "Hello from " . $site_name . ".";

/**
 * shout returns an upper case copy of its argument.
 */
function shout($text)
{
	return strtoupper($text);
}
```

Then include it from the route:

```php
<?php

// @route GET /

include "bootstrap.php";

echo $greeting . "\n";
echo shout($site_name) . "\n";
```

```text
Hello from Hello.
HELLO
```

A top-level `include` shares the includer's scope. `$greeting` and `$site_name` are built in
`bootstrap.php` and they are the route file's variables, with no export step and no
container to fetch them from. That is the whole mechanism, and it is why route files in this
book construct nothing: the prelude builds the graph once and every route reads it by name.

What is shared is the top level of the route file. A function body does not see it, and
`global` parses while doing nothing:

```php
<?php

// @route GET /scope

include "bootstrap.php";

function try_global()
{
	global $site_name;
	return isset($site_name) ? $site_name : "(nothing)";
}

echo "top level: " . $site_name . "\n";
echo "inside a function: " . try_global() . "\n";
```

```text
top level: Hello
inside a function: (nothing)
```

The `global` statement raises nothing and the variable stays unset, so a function relying on
one fails on the value rather than at the point of the mistake. Every function in
`bootstrap.php` therefore takes its collaborators as parameters. `include "bootstrap.php"`
resolves against the application root, so a route file in `routes/` writes the same line as
one at the root.

This chapter uses `bootstrap.php` as a prelude holding a few values. In a full application
it is the composition root: it constructs the connection, the stores, the components and the
shared helpers, in that order, and it is the only file that calls `new` on any of them.
[Structuring an application](25-structuring-an-application.md) builds it.

## Restart after an edit

Stop and restart the server after editing a source file.

The annotation scan runs once, when the server boots, so a new route file, a renamed one or
a corrected annotation is not registered until the next start. Included files are parsed
once and cached by path for the life of the process, so an edit to `bootstrap.php` or to
anything under `src/` is not picked up either.

Templates need the same treatment for a different reason. The engine compiles
`templates/hello.tpl` into `templates/cache/hello.tpl` on first use and recompiles it
whenever the source is newer, but the running server holds the compiled program by path, so
the recompiled file is written to disk and the old one keeps being served:

```text
$ curl -s http://127.0.0.1:8080/tpl
<p>Version one: world</p>

# templates/hello.tpl is edited to say "Version two", then, without a restart:
$ curl -s http://127.0.0.1:8080/tpl
<p>Version one: world</p>

$ head -2 templates/cache/hello.tpl
<?php $_v=&$this->vars;?>
<p>Version two: <?php echo htmlspecialchars($_v['name'], ENT_QUOTES);?>
```

A `.tpl` edit that appears to have no effect is this, not a mistake in the template.

## The two worked applications

Two applications in the repository are the references this book draws from, and both run as
they stand. Start either the way you started yours, after a `composer install`.

`demos/example` is the small one: three endpoints, one table, one template, one static asset
and a venom suite. It is the application `docs/use-cases/application.md` builds, and it is
the right size to read in a sitting.

`demos/common-phpscript` is the complete one: users, groups, permissions, sessions, an admin
panel, 23 routed endpoints, a startup file and an hourly job, over 75 PHP files and 28
fixtures. `bootstrap.php` is its composition root, `routes/` holds one file per endpoint,
`src/` is the composer package `titpetric/phpscript-common` and `schema/` holds the
migrations. It is the application this book builds toward, and every API the later chapters
teach is taken from it.

Next: [Routing and endpoints](10-routing-and-endpoints.md) covers `@route` in full, path
parameters in `$_PATH`, the methods a file may declare, and the `@startup` and `@schedule`
annotations.
