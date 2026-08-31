# Configuration

phpscript can load its runtime configuration from a YAML file. Pass the file
with `-f` (or `--file`) before or after the command:

```bash
phpscript -f config.yml script.php
phpscript -f config.yml run script.php
phpscript server -f config.yml ./my-app
```

Without `-f`, phpscript uses the [`config/config.yml`](../config/config.yml)
compiled into the binary. It does not search the working directory for a
configuration file. A path passed with `-f` must exist and contain valid YAML;
otherwise the command exits with an error.

## Complete example

```yaml
runner:
  work_dir: "."
  writable_paths: []
  autoload: autoload
  upload_max_filesize: 2M
  post_max_size: 8M
  upload_file_mode: "0644"
  memory_limit: 0
  time_limit: 0
  concurrency_limit: 0

flatstack:
  enabled: false

routes:
  enabled: true

server:
  addr: ":8080"
  quiet: false
  modules: []

telemetry:
  enabled: true
  path: "/debug/oida"
  service_name: "phpscript"
  ring_buffer_size: 200
  top_requests: 20
  sample_rate: 100
  track_memory_use: true
  live_stream: true
  driver: memory

document_root: public

autoindex: false

virtualhost: []

env:
  - "PLATFORM_DB_APP=sqlite://app.db"
```

Apart from the `env` entry, this is the embedded
[`config/config.yml`](../config/config.yml) verbatim. That file holds every
default phpscript has; nothing is defaulted in Go. A file passed with `-f` is
read on top of it, so it only has to name the keys it changes, and a section it
leaves out keeps what the embedded file says.

## Runner

`runner` applies to the `run` and `server` commands and to annotated routes
created by the bundled server.

| Key                       | Default    | Purpose                                                                                                                                        |
|---------------------------|-----------:|------------------------------------------------------------------------------------------------------------------------------------------------|
| `work_dir`                |        `.` | Directory inside the runtime source filesystem relative paths resolve against. `chdir()` moves it, per request; this is where each one starts. |
| `writable_paths`          |       `[]` | Directories a script may write to, relative to the application root. An empty list allows every write.                                         |
| `autoload`                | `autoload` | Directory whose classes load on first reference. Absent from the tree, nothing autoloads.                                                      |
| `upload_max_filesize`     |       `2M` | Largest file part a request may carry. A part over it is refused and reported in `$_FILES`.                                                    |
| `post_max_size`           |       `8M` | Largest request body that is parsed at all. A body over it leaves `$_POST` and `$_FILES` empty.                                                |
| `upload_file_mode`        |     `0644` | Mode `move_uploaded_file()` gives a stored upload. Octal, as `chmod()` takes it.                                                               |
| `max_input_vars`          |     `1000` | Fields decoded into `$_GET`, `$_POST` and `$_COOKIE`. The rest are dropped. Negative is no limit.                                              |
| `max_input_nesting_level` |       `64` | Deepest bracket chain a field name may have. A field past it is dropped whole. Negative is no limit.                                           |
| `memory_limit`            |        `0` | Memory one script may hold live, php.ini's. `0` is no limit.                                                                                   |
| `time_limit`              |        `0` | Seconds one script may run, php.ini's `max_execution_time`. `0` is no limit. Not enforced yet.                                                 |
| `concurrency_limit`       |        `0` | Scripts that may run at once. `0` is no limit. Not enforced yet.                                                                               |

### Autoload folder

`autoload` names a directory whose classes load the first time something names
them, with no `include` and no `spl_autoload_register()`. The namespace is the
directory path below it and the class is the file, case for case:

```
myapp/
  autoload/
    Greeter.php          class Greeter
    Acme/
      Thing.php          namespace Acme; class Thing
  public/
    index.php            new Acme\Thing;  — no include
```

The namespace is optional, so the root of the folder holds the classes that
declare none. Only classes resolve this way; PHP has no function autoloading and
phpscript does not invent one, so a file of helpers is still `require`d.

Files in the folder are expected to **declare and nothing else**. Nothing
enforces it, and a top-level `echo` runs the way it runs in any included file —
but it runs at the moment some other file first named a class, which is not a
moment anything can predict.

Two things are worth stating plainly:

- **Keep the folder beside `public/`, never inside it.** A folder below the
  [document root](#document-root) is served over HTTP, which hands out the class
  files as text.
- **There is no key that turns this off.** A tree with no `autoload/` directory
  has no autoloading; the lookup happens once, on a class reference that was
  about to fail anyway. Point the key at another directory to move it.

What the folder loads belongs to the request that loaded it. Registered
autoloaders still come first, so a script that calls `spl_autoload_register()`
or requires composer's `vendor/autoload.php` keeps the resolution order it wrote.
See [Autoloading](reference/namespaces/README.md#autoloading) in the language
reference.

### Writable paths

`writable_paths` names the directories a script may write to. Entries are
relative to the application root, so a site accepting uploads writes:

```yaml
runner:
  writable_paths: ["upload", "public/upload"]
```

`upload` is the project's own directory, reachable by scripts and by nothing
else. `public/upload` is below the document root, so what a script stores there
is served over HTTP at `/upload/...`, including files written while the server
is running. An absolute entry is taken as given, for a host that writes outside
its own tree on purpose.

An empty list, the default, allows every write. A non-empty one is a tree: the
directory and everything below it, and nothing that merely shares its name, so
`upload-old` is not inside `upload`.

Writes refused by the allowlist **throw**, rather than returning `false` like a
write the operating system refused. A script carrying on as though a write
happened is the failure worth stopping, and the exception is catchable:

```php
try {
    move_uploaded_file($file["tmp_name"], "public/upload/" . $name);
} catch (Exception $e) {
    echo "refused: " . $e;
}
```

The functions held to it are the ones that modify the filesystem: `fopen()` in
any mode but `r`, `mkdir()`, `unlink()`, `touch()`, `rename()` at both ends,
`copy()` at the destination, `chmod()`, `chown()`, `chgrp()` and
`move_uploaded_file()`. Reads are untouched: an allowlist says what a script may
change, not what it may look at.

Configuring it also changes what the server does with those directories, which
is the point of naming them:

- **No PHP in a writable directory is executed.** A `.php` file there is served
  as bytes like any other file. A directory a visitor can get content into is
  not a directory to run code from.
- **No `@route` or `@startup` annotation in one is scanned.** Otherwise an
  uploaded file could publish a route the next time the server started.

### Execution limits

`memory_limit` is enforced. Usage is measured by walking the live variables
of every execution frame, so it reflects what the script still holds, not
what it allocated over its lifetime. The walk runs when the script calls
`memory_get_usage()` and at periodic checkpoints while a limit is set;
exceeding the limit raises a `RuntimeException` the script may catch. The
number is an estimate of PHP value payloads, not Go allocator truth, and it
is far below what PHP reports for the same script (no zval overhead).

`time_limit` and `concurrency_limit` are **accepted but not enforced**. The
keys parse and are carried through to the runtime so a configuration written
today keeps working when enforcement lands, rather than failing to load.

`memory_limit` is a size written the way the upload limits are. `time_limit` is
php.ini's `max_execution_time`, in seconds. `concurrency_limit` has no php.ini
equivalent, because there the SAPI owns it; here one process serves several
sites and each gets its own share.

### Sizes

`max_input_vars` and `max_input_nesting_level` are php.ini's, and bound what
the form decoder builds out of an attacker-controlled body: `a[x][x][x]...`
costs one array per level. See
[`$_GET`](reference/predefined-variables/README.md#_get).

`upload_max_filesize` and `post_max_size` are php.ini's, with php.ini's
defaults. A size is written as a bare number of bytes or as a number with an
`M` suffix for megabytes; php.ini's `K` and `G` shorthands are rejected rather
than guessed at, and a value that does not parse fails the configuration file.
`0` means no limit.

```yaml
runner:
  upload_max_filesize: 20M
  post_max_size: 25165824
```

### Upload file mode

An upload arrives in a temporary file readable by nobody but the process that
wrote it, so `move_uploaded_file()` applies a mode when it stores one.
`upload_file_mode` is that mode, written in octal the way `chmod()` takes it,
with or without the leading zero. PHP has no setting for this, it leaves the
mode to the process umask; naming it here means an upload lands the same way
whatever the umask of the process is.

```yaml
runner:
  upload_file_mode: "0640"  # owner writes, group reads, nobody else
```

The default, `0644`, is what a umask of 022 produces and what a web server in
front of the application expects to be able to read. A service that serves
uploads itself, or hands them to a worker in the same group, has no reason to
make them world readable: `0640`, or `0600` for files only this process should
open.

A request over either size limit is not something a script can catch: it happened
before the script started, and all a script sees is an empty superglobal or an
`UPLOAD_ERR_INI_SIZE` entry in `$_FILES`. The reason is reported to the Go host
through `Runtime.RecordError`, which puts it on the request trace and passes it
to a handler installed with `Runtime.OnError`. See
[Errors](reference/errors/README.md#errors-a-script-cannot-catch) and
[`$_FILES`](reference/predefined-variables/README.md#_files).

## Runtime backend

Set `flatstack.enabled` to `true` to select the experimental flat-stack
bytecode backend. Programs outside its native subset transparently use the
compatible runner implementation. See [Flat-stack runtime](./flatstack.md) for
the current native subset and fallback behavior.

## Server

`server` configures the [platform](https://github.com/titpetric/platform) that
`phpscript server` runs on, and applies to that command only.

| Key       | Default | Purpose                                                             |
|-----------|--------:|---------------------------------------------------------------------|
| `addr`    | `:8080` | Address the HTTP server listens on.                                 |
| `quiet`   | `false` | Turn down platform lifecycle logging.                               |
| `modules` |    `[]` | Load only the platform modules named here. An empty list loads all. |

This section, together with `telemetry` below, is the only source of the
platform's options. phpscript builds them from the configuration file, so the
platform's own `PLATFORM_SERVER_ADDR`, `PLATFORM_MODULES` and
`PLATFORM_TELEMETRY_*` environment variables are not read. The `PLATFORM_DB_*`
variables are unrelated to this and still are; see
[Database connections](#database-connections).

## HTTP modules

`routes.enabled` controls whether `phpscript server` recursively scans PHP files
outside `public/` for `// @route` annotations. Static files and directly
requested PHP entrypoints under `public/` are independent of this setting.

## Document root

`document_root` is the directory beneath the application root served over HTTP
by `phpscript server`. It is `public`, and it is not a setting an application is
expected to name: it exists for a tree that already calls that directory
something else.

```yaml
document_root: web
```

A `.php` file found there is executed, anything else is served as a static file,
and a request that names a directory gets that directory's `index.php`, or its
`index.html` when there is no `index.php`. The entrypoint is named relative to
the application root rather than the document root, so an `include` reaching
above the served directory still resolves inside the project. The directory is
also excluded from route scanning, so an annotation in a file that is already
served directly does not publish a second, unguarded route.

It is also where a site's error pages live. A file named after a status,
`404.php`, `503.html`, or `error.php` for the ones a site does not name, is what
answers for it, and writing the file is the whole of turning it on. That is a
convention rather than a setting, and there is no key here for it; see [Error
pages](use-cases/error-handling.md#error-pages) for what a page is given and
which requests get one.

## Autoindex

`autoindex` answers a directory that has no index page with a listing of what is
in it, the way nginx's `autoindex on;` does. It is off:

```yaml
autoindex: true
```

With it off, a directory with no `index.php` and no `index.html` is a 404, and a
site's own [error page](use-cases/error-handling.md#error-pages) answers it like
any other dead link. Publishing every file below the document root is a decision
a site makes, not one it should arrive at by leaving an `index.html` out.

The listing is generated by the server, so there is nothing to write and nothing
to keep in step with the directory. It names each entry, its size and when it
last changed, shows images rather than naming them, and links to the directory
above. Names beginning with a dot are left out. The page carries its own styling
and fetches nothing from anywhere else, so a listing does not tell a third party
what is being browsed.

Virtual hosts each answer for their own: the key is read from the site's
`phpscript.yml`, so one domain can publish a file drop while another does not.
See [Serving static files](use-cases/static-files.md).

## Telemetry

`telemetry.enabled` controls request tracing and the debug front end mounted at
`telemetry.path`. The section is [oida](https://github.com/titpetric/oida)
options, so every field that library documents is accepted here, plus `driver`
and `storage_path`, which phpscript adds.

There is one recorder and the platform owns it: given this section it builds
the tracer, wraps every module it runs in the tracing middleware and mounts the
front end. That is why this is not part of the `server` block above even though
the platform is what consumes it, and why it applies to `phpscript server`
alone. phpscript registers no recorder of its own. It reports interpreter work,
the includes, calls and templates of a request and the spans a script starts
itself, onto the trace that middleware already started, which is why that work
shows up on the same front end as the request that caused it.

The fields that matter for a phpscript service are:

| Key                   | Default                           | Purpose                                                                   |
|-----------------------|----------------------------------:|---------------------------------------------------------------------------|
| `enabled`             |                            `true` | Record traces. When false the middleware passes requests through.         |
| `path`                |                     `/debug/oida` | Mount path of the debug front end.                                        |
| `service_name`        |                       `phpscript` | Name shown in the front end and recorded on every trace.                  |
| `ring_buffer_size`    |                             `200` | Number of completed traces retained for the list and rolling statistics.  |
| `top_requests`        |                              `20` | Maximum trace groups returned by rolling statistics.                      |
| `max_spans_per_trace` |                            `1000` | Spans recorded per trace; further spans are counted and dropped.          |
| `sample_rate`         |                             `100` | Percentage of requests traced, `0` to `100`.                              |
| `track_memory_use`    |                            `true` | Record process-wide allocation and garbage-collection changes per trace.  |
| `live_stream`         |                            `true` | Serve the live view over server sent events instead of a refresh timer.   |
| `ignore_paths`        |                  health endpoints | Paths never traced. Entries ending in `/*` match by prefix.               |
| `driver`              |                          `memory` | Trace store: `memory` (ring buffer) or `disk` (JSON files, restart-safe). |
| `storage_path`        | `/dev/shm/phpscript-trace-detail` | Folder for `driver: disk`. One `{id}.json` per retained trace.            |

Memory tracking has process-wide sampling overhead and concurrent requests can
overlap in those measurements. See [Telemetry](./telemetry.md) for the views,
the representations, and what PHP can record.

A server running [virtual hosts](#virtual-hosts) is the exception to the one
recorder above: each site builds a tracer from its own `telemetry` block and
mounts a front end of its own, and the operator's block is expected to be
disabled.

## Database connections

Each `env` item with a `PLATFORM_DB_<NAME>=<driver>://<dsn>` key registers a
named connection for `Database`. Names are lowercased, so this config:

```yaml
env:
  - "PLATFORM_DB_APP=sqlite://app.db"
  - "PLATFORM_DB_REPORTING=postgres://user:pass@db/reporting?sslmode=disable"
```

is available to PHP as:

```php
$app = new Database("app");
$reporting = new Database("reporting");
```

The list is passed to the database connection registry; it does not add the
entries to the process environment or PHP variables. Actual process environment
variables named `PLATFORM_DB_*` are also registered when phpscript starts.

## Virtual hosts

`virtualhost` lists the sites one `phpscript server` answers for, one entry per
site. While the list is empty the server serves a single application root, the
one named on the command line. While it is not, the `Host` header selects the
site, and an application root on the command line is an error: the entries name
their own roots and a further one has no site to belong to.

| Key             | Default  | Purpose                                                                                     |
|-----------------|---------:|---------------------------------------------------------------------------------------------|
| `domain`        | required | The `Host` headers this entry answers for, separated by spaces.                             |
| `root`          | required | The application root, the directory holding the site's `phpscript.yml`.                     |
| `document_root` | `public` | Directory beneath `root` served over HTTP. It wins over the site's own `document_root` key. |

A site answering to more than one name lists them all in `domain`. They are
names for one site, not copies of it: the site is built once, and every name
shares its routes, its recorder and its connections:

```yaml
virtualhost:
  - domain: example.com www.example.com
    root: /srv/example
```

The first name is the site's own. It is what errors report the site as, and what
the names of the modules it registers carry.

Listing the same name twice, in one entry or across two, is a startup error: the
second would silently win.

```yaml
telemetry:
  enabled: false

env: []

virtualhost:
  - domain: shop.example.com
    root: /srv/shop
  - domain: blog.example.com
    root: /srv/blog
```

The entry says which domain reaches a site and where it lives; everything else
about the site comes from its own file. [Virtual
hosting](use-cases/virtual-hosting.md) works a two site server through in full.

### The site's configuration file

Each `root` must hold a `phpscript.yml`. It is read on top of the configuration
passed with `-f` the same way that file is read on top of the embedded defaults,
so it only names what it changes and inherits the rest. A `root` without one
fails startup rather than serving the site under settings its author never
wrote.

A site may not set `server` or `virtualhost`: the listen address belongs to the
operator, and a site holds no sites. Either key is a startup error rather than a
silently dropped block, so a site author is never left believing they moved the
listen address:

```text
virtualhost "shop.example.com": /srv/shop/phpscript.yml: "server" is set by the operator, not by the site
```

### Host matching

A `Host` header is compared lowercased, without its port and without a trailing
dot, against domains normalized the same way. `shop.example.com`,
`SHOP.Example.com.` and `shop.example.com:8080` all reach the same site. There
are no wildcards and no default site: a `Host` no entry claims gets 404 and
never reaches a site's code.

Each site is a router of its own, so its `@route` endpoints and its document
root answer on its own domain and nowhere else. Its `@startup` and `@schedule`
jobs run on its behalf. The platform modules those jobs run as are named per
site, `phpstartup:shop.example.com` and `phpschedule:shop.example.com`, so
`server.modules` can still name one site's modules.

### Telemetry per site

A site's `telemetry` block builds its own tracer and mounts its own debug front
end inside its own router, so the dashboard and the traces on it belong to that
domain alone.

Turn the operator's own `telemetry` off when running virtual hosts. The platform
mounts its dashboard on the root router, in front of the host mux, so that path
prefix answers on every domain, including one no entry claims, and shadows the
front end every site mounts under it. A site that names the operator's path
explicitly is told so at startup:

```text
virtualhost "shop.example.com": telemetry path "/debug/oida" is the path the server mounts its own dashboard on
```

The check only fires for a path the site's file names itself. A site that names
none is not asking for one and is left alone, which is why turning the
operator's block off is what makes the site dashboards reachable.

Two sites running `driver: disk` may not share a `storage_path`, or their traces
would land in one store.

### Databases per site

A site's connections are built from its own `env`, and the provider holding them
sees nothing else, so a site can open the connections its own file names and no
others. `new Database("shop")` on a site that did not configure `shop` fails
with `no configuration found for database: [shop]`.

`env` is a list, and the overlay replaces a list wholesale rather than appending
to it. A site that declares an `env` of its own therefore gets only its own
connections, while a site that declares none inherits every connection the
operator configured. The shipped `config/config.yml` carries one `PLATFORM_DB_*`
entry, so setting `env: []` in the operator's file is the way to run virtual
hosts with no shared connections.

### The environment per site

`env` is also what the site's scripts read with `getenv()`, and it is all they
read: a site is handed the environment it declared rather than the process
environment, so `getenv()` cannot be used to find out what the operator, or
another site, was started with.

Variables named `PLATFORM_*` are the exception in the other direction. They
configure phpscript and the platform it runs on, connection strings included, so
they are never visible to a script even when the site declared them itself. A
site that lists `PLATFORM_DB_SHOP` gets the connection and reads nothing from
`getenv("PLATFORM_DB_SHOP")`.

A single application server, one with no `virtualhost` entries, keeps the
process environment plus whatever `env` adds, minus the same `PLATFORM_*`
variables.

### Startup checks

The whole list is loaded and checked before the server listens, so a broken
entry fails startup rather than one request:

| Check                                                      | Failure |
|------------------------------------------------------------|---------|
| `domain` and `root` are set                                | error   |
| No host is named twice, in one `domain` or across two      | error   |
| `root` exists and is a directory                           | error   |
| `root/phpscript.yml` exists and parses                     | error   |
| The site does not set `server` or `virtualhost`            | error   |
| The document root exists beneath `root`                    | error   |
| Two `driver: disk` sites do not share a `storage_path`     | error   |
| A site does not claim the operator's telemetry path        | error   |
| An application root is not also passed on the command line | error   |

### Startup jobs per site

A site's `@startup` jobs are its own. A job that fails is recorded on that
site's recorder as a background trace and the server carries on, because the
alternative is one site's broken job stopping every other site in the process.
The remaining jobs of that site still run: they are independent, and every
failure is reported rather than only the first.

A single application server, one with no `virtualhost` entries, keeps a failing
`@startup` fatal. There is no other tenant to protect, and a process that came
up with its schema unapplied is worse than one that did not come up.
