# `phpscript server [directory]`

Run the PHP application rooted at the given directory. The directory defaults
to the current working directory and must contain a `public/` web root.

It accepts the [global flags](README.md#global-flags) and nothing else: `-f`,
`-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and
`--coverfile`.

```bash
phpscript server
phpscript server ./my-app
```

CSS, JavaScript, images, and other non-PHP files are served directly from
`public/`. PHP files in `public/` are executable by filename, and a request that
names a directory resolves to its `index.php`, or to its `index.html` when there
is no `index.php`: `/` is `public/index.php` or `public/index.html`, and `/docs/`
is `public/docs/index.php` or `public/docs/index.html`. A directory with neither
is a 404 unless the configuration sets `autoindex: true`, which answers it with a
generated listing instead. Files outside `public/` are never directly exposed.
See [Serving static files](../use-cases/static-files.md).

A file in `public/` named after a status answers for it: `public/404.php` is
what a visitor following a dead link sees, and `public/503.php` answers a
`throw new Exception("...", 503)`. Writing the file is all there is to it, and a
site with no such file answers the way it did before. Programs are not sent one:
a `fetch()`, an API client and curl get the plain status, as does any endpoint
that wrote a body or declared a `Content-Type`. A page answering for a request
that matched nothing gets that request whole, `$_POST` and `$_FILES` included,
which is what lets `404.php` dispatch a site's own routes. See [Error
pages](../use-cases/error-handling.md#error-pages).

PHP files outside `public/` are scanned recursively for
`// @route METHOD /path/{param}` annotations. Those routes execute with the
application directory as their source filesystem, so they can include shared
bootstrap code and templates outside the web root. Route annotations in
`public/` are ignored. Route loading is controlled by the active configuration:

```yaml
routes:
  enabled: true
```

Before the server starts listening, PHP files anywhere in the application tree
that contain an `@startup` comment execute once in path order. Use these files
for required setup such as database migrations:

```php
<?php
// @startup

$migrate = new Database\Migrate("app");
$migrate->load("./schema/*.up.sql");
$migrate->run();
```

`@schedule` annotations start after listen and keep running until shutdown:

```php
<?php
// @schedule daily -- prune
// @schedule every 5 minutes -- sync

switch ($argv[1]) {
case "prune":
	break;
case "sync":
	break;
default:
	echo "Unknown/missing command";
	exit(1);
}
```

Specs: `every N seconds|minutes|hours`, `hourly`, `daily`, `weekly`,
`monthly`, `every weekday`, `every sunday` (any weekday), `N times per hour|day`.
Arguments after `--` become `$argv[1:]`. Calendar specs fire at local midnight.
A tick is skipped if the previous run is still going. Output is stored on the
oida span as `output`.

Startup files run with the CLI SAPI and the application filesystem. If one
fails, the server is not started. See
[Run migrations](../use-cases/database.md#run-migrations) for the schema layout
these files apply.

```text
my-app/
├── public/
│   ├── index.php
│   └── assets/
│       ├── app.js
│       └── style.css
├── routes/
│   └── users.php
└── templates/
    └── user.tpl
```

See [PHP routing](../use-cases/routing.md) for route annotation details.

## Coverage from a running server

`--cover` counts the statements the server executes, across every request, every
routed endpoint, every `@startup` job and every scheduled run, for the life of
the process. A request's counts are folded into one aggregator when it ends, so
what the process holds is one entry per statement range rather than one per
parsed program.

```bash
phpscript --cover --coverfile cover/site.{time}.cov server
```

The profile is written when the server shuts down gracefully, with `{time}`
expanded and the directory created. A test flow that cannot wait for that, or
that runs the server in a container with nothing writable, reads it off the
process instead:

```bash
curl http://localhost:8080/debug/phpscript/coverage              # the profile
curl "http://localhost:8080/debug/phpscript/coverage?mode=file"  # per file
curl "http://localhost:8080/debug/phpscript/coverage?mode=func"  # per function
```

The modes are `--cover`'s: `line` is the profile itself and is the default,
`func` and `file` are the report `go tool cover -func` prints, which
`summary coverfunc --packages` and `--files` fold per folder and per file. The
endpoint is registered only when `--cover` is set.

Counting turns the flatstack backend off for the process: coverage is an
interpreter feature and the fallback is atomic, so a counted program runs
interpreted whole. A server measuring coverage is not a server measuring
throughput.

The dbadmin demo is wired this way end to end. `compose.yml` starts it with
`--cover`, the venom suite in `demos/dbadmin/tests` reads the endpoint after it
runs, and `atkins gen:coverage:dbadmin` renders
[docs/coverage/dbadmin.md](../coverage/dbadmin.md) from it.
