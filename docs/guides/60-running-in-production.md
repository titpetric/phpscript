# Running in production

This chapter covers the operator's side of the application: how a configuration file is
layered over the defaults, how one server answers for several domains, what a script is
allowed to write, what the server records about itself, and what a deployment consists
of. At the end of it you can put the application from the previous chapters behind a
domain and say which failures happen at startup and which reach a request.

## How to layer a configuration file

`phpscript` has one set of defaults, and they live in
[`config/config.yml`](../../config/config.yml) compiled into the binary. A file passed
with `-f` is unmarshalled on top of that already populated structure, so it only names
what it changes. Run `phpscript -f config.yml server .` and nothing else is searched
for: without `-f` the embedded file is the whole configuration, and a `config.yml`
sitting in the working directory is read by nobody.

The application from the earlier chapters ships this file, and it is the whole of it:

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

Every key it leaves out keeps the embedded value. There are nine top level keys:

| Key             | Covers                                                                     |
|-----------------|----------------------------------------------------------------------------|
| `runner`        | The interpreter: working directory, writable paths, upload and size limits |
| `flatstack`     | Selects the experimental flat bytecode backend                             |
| `routes`        | Whether `@route` annotations outside the document root are scanned         |
| `server`        | Listen address, lifecycle logging, which platform modules load             |
| `telemetry`     | Request tracing and the debug front end                                    |
| `document_root` | The directory beneath the application root served over HTTP                |
| `autoindex`     | Answer a directory with no index page with a listing                       |
| `virtualhost`   | The sites this server routes by domain                                     |
| `env`           | The environment scripts read, and the `PLATFORM_DB_*` connections          |

`env` is a list, and the overlay replaces a list wholesale. A file that names `env` gets
its own entries and none of the embedded ones, which matters because the embedded file
carries a `PLATFORM_DB_SQLITE_TEST` entry.

The full key reference is [Configuration](../configuration.md).

## How to run several applications from one server

`virtualhost` lists the sites one server answers for. Each entry gives the `Host` headers
that reach the site, the application root, and optionally the directory beneath it that
is served:

```yaml
server:
  addr: "127.0.0.1:8080"

telemetry:
  enabled: false

env: []

virtualhost:
  - domain: shop.example.com
    root: shop
  - domain: blog.example.com
    root: blog
```

While the list is empty the server serves one application root, the one named on the
command line. While it is not, an application root on the command line is an error: the
entries name their own roots.

Each `root` must hold a `phpscript.yml`. It is read on top of the operator's file the
same overlay way, so it names only what it changes, and a root without one fails startup
rather than serving the site under settings its author never wrote:

```yaml
# shop/phpscript.yml
telemetry:
  enabled: true
  service_name: "shop"

env:
  - "PLATFORM_DB_SHOP=sqlite://shop.db"
```

Two keys are the operator's, listed in `config/virtualhost.go` as `tenantKeys`: `server`
and `virtualhost`. A site that names either is a startup error rather than a block that
is silently dropped, so a site author is never left believing they moved the listen
address or nested a site of their own:

```text
virtualhost "shop.example.com": /srv/shop/phpscript.yml: "server" is set by the operator, not by the site
```

After unmarshalling, the loader restores the operator's `Server` and clears
`VirtualHost` anyway, which covers a configuration assembled in Go rather than read from
a file.

`env` is per site and, being a list, replaces wholesale. A site's `getenv()` sees only
what its own file declared, so it cannot read what the operator or another site was
started with. Databases resolve from the same list: `new Database("shop")` on a site that
did not configure `shop` fails with `no configuration found for database: [shop]`.
Setting `env: []` in the operator's file is what keeps the embedded connection out of
every site that declares none.

Host matching is exact. The header is compared lowercased, without its port and without a
trailing dot, against domains normalized the same way. There are no wildcards and no
default site: a `Host` no entry claims gets a 404 and never reaches a site's code.

`Config.ValidateVirtualHosts()` runs before the server listens and loads every entry,
because the document root and the telemetry block it checks come from the site's own
file. A duplicate domain, a missing root, a missing `phpscript.yml`, a tenant key, a
document root that is not there, two `driver: disk` sites sharing a `storage_path`, or a
site claiming the operator's telemetry path all fail startup rather than one request.
[Virtual hosting](../use-cases/virtual-hosting.md) works a two site server through in
full.

## How to confine what a script may write

`runner.writable_paths` is a list of paths, relative to the application root unless an
entry is absolute. An entry is a tree: the path and everything below it, and nothing that
merely shares its name, so `upload-old` is not inside `upload`. An empty list, the
default, allows every write. A non-empty one refuses every write outside it, as the two
entries in the file above do.

A refused write throws rather than returning `false`, because a script carrying on as
though a write happened is the failure worth stopping:

```php
<?php
try {
	$f = fopen("notes.txt", "w");
} catch (Exception $e) {
	// refused: fopen(notes.txt): writable_paths allows templates/cache, common.db
	echo "refused: " . $e->getMessage() . "\n";
}
```

The functions held to the list are the ones that change the filesystem: `fopen()` in any
mode but `r`, `mkdir()`, `unlink()`, `touch()`, `rename()` at both ends, `copy()` at the
destination, `chmod()`, `chown()`, `chgrp()` and `move_uploaded_file()`. Reads are
untouched.

The template compile cache has to be in the list. `MiniTPL\Template` writes the compiled
form of every template it renders under the directory `set_compile_location()` names, so
the first page render fails on a server that turned the allowlist on and left the cache
directory out of it. In the application from
[Templates and rendering](20-templates-and-rendering.md) that directory is
`templates/cache`, which is why it is the first entry above.

Naming a directory also changes what the server does with it. No `.php` file in one is
executed: it is served as bytes like any other file, so an upload directory below the
document root cannot turn an upload form into a way to run code. `@route` annotations in
one are not scanned either, so an uploaded file cannot publish a route on the next start.

## What the limit keys do

`runner/options.go` carries three execution limits, and one of them runs:

| Key                 | Enforced | Behaviour                                                                  |
|---------------------|----------|----------------------------------------------------------------------------|
| `memory_limit`      | yes      | A script over it gets a catchable `RuntimeException`                       |
| `time_limit`        | no       | Parsed and carried into `runner.Options.TimeLimit`, read by nothing        |
| `concurrency_limit` | no       | Parsed and carried into `runner.Options.ConcurrencyLimit`, read by nothing |

`memory_limit` measures what a script still holds by walking the live variables of every
execution frame, at checkpoints of 256 statements or 4096 VM instructions and whenever
the script calls `memory_get_usage()`:

```text
$ phpscript -f memory.yml run big.php
Unexpected error: Allowed memory size of 1048576 bytes exhausted (1049244 bytes in use)
```

The number is an estimate of PHP value payloads rather than Go allocator truth, and it
is well below what PHP reports for the same script.

The doc comments on `TimeLimit` and `ConcurrencyLimit` both say `NOT ENFORCED YET`, and a
grep over the tree finds no reader for either. They parse so a configuration written today
keeps loading when enforcement lands.

For capacity planning that means two things. A request that loops forever occupies a
goroutine until the process is restarted, so the reverse proxy owns the request timeout
and the server has no second opinion about it. The number of scripts running at once is
the number of connections the platform accepts, so one expensive endpoint under load
competes with every cheap one in the same process. The design work for per-endpoint bounds
is in [`proposal-annotation-limits.md`](../../demos/common-report/proposal-annotation-limits.md),
which proposes `@concurrency N`, `@timeout 5s` and `@body max=100M` and records which of
the three can be built without an interpreter change.

## How to read what the server is doing

The `telemetry:` block configures request tracing and the debug front end. The embedded
defaults turn it on at `/debug/oida` under the service name `phpscript`, retaining 200
completed traces, sampling every request, tracking memory use and streaming the live
view.

There is one recorder and the platform owns it. Given this block it builds the tracer,
wraps every module it runs in the tracing middleware, and mounts the front end under
`path`. phpscript registers no recorder of its own: it observes the interpreter onto the
trace that middleware already started, which is why an include, a call, a template and a
query appear on the same timeline as the request that caused them.

The front end serves five views under `path`: the domains this process serves, the live
view, the retained traces, rolling statistics, and one trace by id. Each request carries
a ULID in its `Request-Id` request and response headers, and that id is the trace id, so
a log line links straight to `/debug/oida/trace/{id}`.

Two drivers store the traces. `memory` is a ring buffer of `ring_buffer_size` completed
traces and loses them on restart. `disk` writes one `{id}.json` per retained trace under
`storage_path`, defaulting to `/dev/shm/phpscript-trace-detail`, so a crash leaves the
traces that led up to it readable. Sampling still decides what is recorded, and the disk
driver persists what was sampled and nothing more.

Turn the operator's own block off when running virtual hosts. The platform mounts its
dashboard on the root router, in front of the host mux, so that path prefix answers on
every domain and shadows the front end each site mounts for itself. Each site builds its
own tracer from its own `telemetry` block, so the dashboard and the traces on it belong
to that domain alone. See [Telemetry](../telemetry.md).

## Where files are served from

`document_root`, `public` by default, is the only directory served directly. A `.php`
file there is executed by filename, anything else is served as bytes, and a request
naming a directory gets its `index.php` or its `index.html`. Everything above it,
`phpscript.yml` and the `routes/`, `src/`, `schema/` and `templates/` directories among
them, has no URL and is not reachable by one.

`@route` annotations under the document root are ignored: the scanner is handed the
document root as an excluded directory, so a file already served by name does not also
publish a second, unguarded route. Route files belong outside it, which is what `routes/`
in the application layout is for.

`@startup` and `@schedule` are scanned everywhere. The exclusion applies to route
scanning alone, so a `public/boot.php` carrying `@startup` runs at boot, and so does one
in a writable directory. Keep both annotations out of the document root.

## How migrations run at boot

An `@startup` file runs once before the server listens, in path order, with the CLI SAPI.
Migrations belong there, so a request never meets a half applied schema:

```php
<?php

// @startup

$migrate = new Database\Migrate("common");
$migrate->load("./schema/*.up.sql");
$migrate->run();

echo "migrate: schema applied\n";
```

`run()` records what it applied in a `migrations` table, keyed by the connection name, so
a later start skips the statements that already ran.

On a single application server a failing `@startup` aborts startup: a process that came
up with its schema unapplied is worse than one that did not come up. Under virtual hosts
the failure is recorded on that site's recorder as a background trace and the server
carries on, because the alternative is one site's broken job stopping every other site in
the process. The remaining jobs of that site still run.

Migration files are append only. Progress is recorded per statement index within a file,
so a later schema change is a new statement at the end of the file that owns the table:

```sql
-- schema/user_session.up.sql, the statements that already ran
CREATE TABLE IF NOT EXISTS user_session (
	id CHAR(26) PRIMARY KEY NOT NULL,
	token VARCHAR(32) NOT NULL,
	user_id CHAR(26) NOT NULL,
	expires_at DATETIME NOT NULL,
	revoked_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS uidx_user_session_token ON user_session(token);

-- appended later, at the end of the same file
ALTER TABLE user_session ADD COLUMN last_ip VARCHAR(45) NOT NULL DEFAULT '';
```

Running the migration again after the append applies the new statement and nothing else:
the `migrations` row for the file moves from `statement_index` 1 to 2 and a third run
changes nothing.

Editing a statement that already ran changes nothing on a database that applied it and
gives a fresh database a different schema from the running one. Renaming or reordering
files has the same effect, because a statement is recorded under its position in the
file. Add at the end and leave what is above it alone.

## What the scheduler guarantees

A `@schedule` job starts after the server listens and runs until shutdown. The overlap
lock is a `sync.Mutex` held inside the job's own goroutine, so it skips a tick while the
previous run of that job in that process is still going, and it knows nothing about any
other process. Two servers over one application tree both tick, and neither sees the
other:

```php
<?php
// @schedule hourly
include "bootstrap.php";
echo "session-prune: removed " . $session->prune() . " sessions\n";
```

That job is safe to run twice because the delete is idempotent. A job that must not run
twice needs its own lease row: a table the job updates with a compare-and-set on an
owner and an expiry, taken before the work and released after it. The scheduler will not
provide one.

A restart resets the phase of an interval spec. `hourly`, `every 5 minutes` and
`4 times per hour` schedule their first run one interval after the process started, so a
server restarted every twenty minutes never reaches an hourly job. Calendar specs are not
affected: `daily`, `weekly`, `monthly`, `every weekday` and `every sunday` fire at local
midnight and a restart moves nothing.

## Why a deployment is a restart

The server reads PHP at boot and keeps what it read:

- The route table is built once, when the annotation module mounts. A new `@route` line,
  a new route file, or a deleted one changes nothing until the process starts again.
- `@startup` and `@schedule` job sources are read into memory during the scan. Editing a
  job body has no effect on the running process.
- Included files are parsed once and cached by cleaned path for the life of the process.
  `bootstrap.php` and everything under `src/` are included, so a change to a class or to
  the composition root is not picked up.
- Compiled templates are cached by path twice over. `MiniTPL\Template` recompiles a
  template whose source is newer than its compiled form, then includes the compiled file,
  and the include cache returns the parse it already has for that path.

Deploy by putting the new tree in place and restarting the process. There is no reload
signal and no opcache invalidation call.

## A deployment checklist

- Pass the configuration with `-f`; nothing is read from the working directory.
- Set `env` explicitly, `env: []` included, because a list replaces rather than merges.
- Give every virtual host root a `phpscript.yml` without `server` or `virtualhost` in it.
- Turn the operator's `telemetry` off under virtual hosts, and give each `disk` site its
  own `storage_path`.
- List the template compile cache and every upload directory in `runner.writable_paths`,
  and nothing else.
- Keep `@route`, `@startup` and `@schedule` files out of the document root and out of the
  writable paths.
- Set `memory_limit`, the only execution limit that runs, and give the reverse proxy the
  request timeout that `time_limit` does not enforce.
- Run migrations from an `@startup` file, appending to a migration file rather than
  editing it.
- Give a scheduled job that must not run twice a lease row before running a second server
  over the same tree.
- Restart to deploy, and check the startup log for the module list and the route count
  before sending traffic.

Next: [Resources](99-resources.md) collects the reference documentation, the worked
applications and the packages this book builds on.
