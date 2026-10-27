# `phpscript server [directory]`

Run the PHP application rooted at the given directory. The directory defaults to the current working directory and must contain a `public/` web root.

It accepts the [global flags](README.md#global-flags): `-f`, `-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`.

```bash
phpscript server
phpscript server ./my-app
```

The server serves CSS, JavaScript, images, and other non-PHP files directly from `public/`. PHP files in `public/` are executable by filename, and a request that names a directory resolves to its `index.php`, or to its `index.html` when there is no `index.php`: `/` is `public/index.php` or `public/index.html`, and `/docs/` is `public/docs/index.php` or `public/docs/index.html`. A directory with neither is a 404 unless the configuration sets `autoindex: true`, which answers it with a generated listing instead. Files outside `public/` are never directly exposed. See [Serving static files](../use-cases/static-files.md).

A file in `public/` named after a status answers for it: `public/404.php` answers a visitor following a dead link, and `public/503.php` answers a `throw new Exception("...", 503)`. Writing the file is all there is to it, and a site with no such file answers the bare status. Programs are not sent one: a `fetch()`, an API client and curl get the plain status, as does any endpoint that wrote a body or declared a `Content-Type`. A page answering for a request that matched nothing gets that request whole, `$_POST` and `$_FILES` included and `404.php` can dispatch a site's own routes. See [Error pages](../use-cases/error-handling.md#error-pages).

The server scans PHP files outside `public/` recursively for `// @route METHOD /path/{param}` annotations. Those routes execute with the application directory as their source filesystem and they can include shared bootstrap code and templates outside the web root. The scan ignores route annotations in `public/`. The active configuration controls route loading:

```yaml
routes:
  enabled: true
```

Before the server starts listening, PHP files anywhere in the application tree that contain an `@startup` comment execute once in path order. Use these files for required setup such as database migrations:

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

Specs: `every N seconds|minutes|hours`, `hourly`, `daily`, `weekly`, `monthly`, `every weekday`, `every sunday` (any weekday), `N times per hour|day`. Arguments after `--` become `$argv[1:]`. Calendar specs fire at local midnight. The scheduler skips a tick while the previous run is still going. Each run stores its output on the oida span as `output`.

Startup files run with the CLI SAPI and the application filesystem. If one fails, the server is not started. See [Run migrations](../use-cases/database.md#run-migrations) for the schema layout these files apply.

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

## Testing a configuration

`phpscript -t` reports whatever would stop a server from starting, and exits non-zero when there is anything:

```bash
phpscript -f config.yml -t
```

```text
config.yml: ok
```

```text
virtualhost "shop.example.com": document root: stat /srv/shop/public: no such file or directory
config.yml: failed
```

It runs the [startup checks](../configuration.md#startup-checks) against every virtual host, and for a single application root checks that the root and its document root exist, which the server itself does not: `fs.Sub` does not stat and a missing `public/` is otherwise a 404 on the first request. It also reads the `mail` and `test` blocks, which the server never reaches.

It binds no socket, runs no `@startup` job, opens no connection and creates no trace storage. A `telemetry.driver: disk` under test does not get its directory made, because a command that tests a configuration must not leave one behind owned by whoever ran the test.

## Reloading

`phpscript -s reload` re-reads the configuration without dropping connections:

```bash
phpscript -f config.yml -s reload
```

It reads `server.pid_file` from the server's own configuration and sends that process a `SIGHUP` and it needs the same `-f` and `-w`. `kill -HUP $(cat /run/phpscript.pid)` does the same thing. Success prints nothing; what the reload did is in the server's log.

The server holds the socket across the reload and the address survives and whichever generation takes a queued connection serves it. The reload rebuilds everything above the socket from the file on disk now: the virtual host list, each site's `phpscript.yml`, the routes, and the `@startup` jobs, which run again.

That rebuild includes the caches and a reload is also how the server picks up an edited `.php` file under [`runner.precompile`](../configuration.md#precompilation). The server reads no modification time; the new generation walks each site's tree and the old one's parsed programs go with it.

The server refuses a configuration it cannot use before tearing anything down, and goes on serving the sites it already holds:

```text
ERROR reload refused, still serving error="reload refused: virtualhost \"c.example.com\": root: stat /srv/c: no such file or directory"
```

That check is the same one `-t` runs and `-t` passing is a good predictor that a reload will apply. `-t` is still worth running first: it reports to the terminal, and a signal has no way to answer back.

The server reads these settings once and a reload cannot change them: `server.addr`, `server.quiet`, `server.modules`, `server.pid_file` and the operator's `telemetry` block. A reload that changes one of them logs which, and keeps the value it started with. Those need a restart.

A `SIGINT` or a `SIGTERM` stops the server instead: it stops accepting, finishes what is in flight, stops the modules in registration order and removes the pidfile.

## Coverage from a running server

`--cover` counts the statements the server executes, across every request, every routed endpoint, every `@startup` job and every scheduled run, for the life of the process. A request folds its counts into one aggregator when it ends and the process holds one entry per statement range, with no second entry per parsed program.

```bash
phpscript --cover --coverfile cover/site.{time}.cov server
```

The server writes the profile when it shuts down gracefully, with `{time}` expanded and the directory created. A test flow that cannot wait for that, or that runs the server in a container with nothing writable, reads it off the process instead:

```bash
curl http://localhost:8080/debug/phpscript/coverage              # the profile
curl "http://localhost:8080/debug/phpscript/coverage?mode=file"  # per file
curl "http://localhost:8080/debug/phpscript/coverage?mode=func"  # per function
```

The modes are `--cover`'s: `line` is the profile itself and is the default, `func` and `file` are the report `go tool cover -func` prints, which `summary coverfunc --packages` and `--files` fold per folder and per file. The server registers the endpoint only under `--cover`.

Counting turns the flatstack backend off for the process: coverage is an interpreter feature and the fallback is atomic and a counted program runs interpreted whole. A server measuring coverage is not a server measuring throughput.

The dbadmin demo runs this way end to end. `compose.yml` starts it with `--cover`, the venom suite in `demos/dbadmin/tests` reads the endpoint after it runs, and `atkins gen:coverage:dbadmin` renders the [dbadmin coverage report](https://github.com/titpetric/phpscript/releases/latest/download/dbadmin.md) from it.
