# Configuration

phpscript can load its runtime configuration from a YAML file. Pass the file with `-f` (or `--file`) before or after the command:

```bash
phpscript -f config.yml script.php
phpscript -f config.yml run script.php
phpscript server -f config.yml ./my-app
```

Without `-f`, phpscript uses the [`config/config.yml`](../config/config.yml) compiled into the binary. It does not search the working directory for a configuration file. A path passed with `-f` must exist and contain valid YAML. Otherwise the command exits with an error.

Two commands read a second file called `phpscript.yml`, discovered by its position in the tree. A [virtual host](#virtual-hosts) reads the file in the application root its entry points name. [`phpscript test`](#test-suites) reads those it finds in the fixture tree. Both layer over whatever `-f` produced, as that file layers over the embedded defaults.

## Complete example

```yaml
runner:
  work_dir: "."
  runtime: phpscript
  precompile: true
  trace_level: info
  writable_paths: []
  include: ""
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

test:
  include: ""
  hooks:
    setup: ""
    teardown: ""
  parallel: 1
  cache: worker
  skip_php: false

server:
  addr: ":8080"
  quiet: false
  modules: []
  pid_file: ""

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

mail: {}

document_root: public

autoindex: false

virtualhost: []

env:
  - "PLATFORM_DB_APP=sqlite://app.db"
```

Apart from the `env` entry, this is the embedded [`config/config.yml`](../config/config.yml) verbatim. That file holds every default phpscript has. Go defaults nothing. A file passed with `-f` layers on top and names only the keys it changes. A section it leaves out keeps the embedded file's value.

## Runner

`runner` applies to the `run` and `server` commands and to annotated routes created by the bundled server.

| Key                       | Default     | Purpose                                                                                                                                        |
|---------------------------|------------:|------------------------------------------------------------------------------------------------------------------------------------------------|
| `work_dir`                |         `.` | Directory inside the runtime source filesystem relative paths resolve against. `chdir()` moves it, per request; this is where each one starts. |
| `runtime`                 | `phpscript` | Namespace the runtime installs: `phpscript` installs `areas`, `go` installs `Runtime\Go` alone and reads every file as an import of it.        |
| `precompile`              |      `true` | Parse and compile the source tree at startup rather than per request. See [Precompilation](#precompilation).                                   |
| `trace_level`             |      `info` | How much of its work the interpreter reports: `off`, `info` or `debug`. See [Telemetry](./telemetry.md#trace-level).                           |
| `writable_paths`          |        `[]` | Directories a script may write to, relative to the application root. An empty list allows every write.                                         |
| `include`                 |        `""` | File included ahead of every entrypoint, once per request, when it exists. `--include` overrides it.                                           |
| `upload_max_filesize`     |        `2M` | Largest file part a request may send. `$_FILES` reports a larger part as refused.                                                              |
| `post_max_size`           |        `8M` | Largest request body the decoder reads. A larger body leaves `$_POST` and `$_FILES` empty.                                                     |
| `upload_file_mode`        |      `0644` | Mode `move_uploaded_file()` sets on a stored upload. Octal, as `chmod()` takes it.                                                             |
| `max_input_vars`          |      `1000` | Fields decoded into `$_GET`, `$_POST` and `$_COOKIE`. The decoder drops the rest. Negative is no limit.                                        |
| `max_input_nesting_level` |        `64` | Deepest bracket chain a field name may have. The decoder drops a deeper field whole. Negative is no limit.                                     |
| `memory_limit`            |         `0` | Memory one script may hold live, php.ini's. `0` is no limit.                                                                                   |
| `time_limit`              |         `0` | Seconds one script may run, php.ini's `max_execution_time`. `0` is no limit. Not enforced yet.                                                 |
| `concurrency_limit`       |         `0` | Scripts that may run at once. `0` is no limit. Not enforced yet.                                                                               |

The `runtime` key selects the whole namespace. `phpscript` is the default and installs what `areas` names. `go` installs only `Runtime\Go`. It adds no area under `Runtime\PS` or `Runtime\PX` and registers no PHP throwable class: try/catch speaks go's `error`. The parser reads every file as though it wrote `use Runtime\Go;`: go's builtins, `[]T` and `import "pkg"`. The configuration refuses a non-empty `areas` beside `runtime: go`.

### Precompilation

`precompile` decides when phpscript parses and compiles a `.php` file. With it on, `phpscript server` walks the application root before it accepts a request. It parses every `.php` file below the root and compiles each program. A request reads both back out of the caches the site shares. With precompile off, the first request to reach a file parses it. Each later request re-parses an entrypoint, because the caches answer only for `include` and `require`.

Parsing is half the work. Flat bytecode keys on the parsed program, as do the interpreter's compiled expressions. A re-parsed entrypoint discards both and rebuilds them for the request. Precompilation keeps one AST per file for the life of the process. Both caches then keep their entries across requests.

```yaml
runner:
  precompile: false
```

The runtime invalidates no cache and reads no modification time. [A reload or a restart](cli/server.md#reloading) picks up an edited file. The memory a tree parses to stays resident from startup. It does not grow as requests reach files. A process serving a few megabytes of PHP starts at the whole tree's parse size.

The walk parses files in parallel, `GOMAXPROCS` at a time. It skips a file that does not parse. It also skips a program the bytecode compiler refuses. Startup reports no source error. A request naming such a file gets the same answer it gets with precompilation off. One broken file below the root cannot stop the server coming up.

A virtual host sets `precompile` in the `runner` block of `phpscript.yml`, then precompiles its tree. One site paying the startup cost says nothing about another.

Only `phpscript server` walks a tree. The include cache it fills holds 10,000 programs. A larger tree keeps whichever 10,000 the walk left. It parses the rest when a request arrives.

### Include

`include` names a file pulled in ahead of every entrypoint, once per request, for the functions and classes every entrypoint in an application needs. The entrypoint and its includes see its declarations. A composer autoloader is the usual one:

```yaml
runner:
  include: vendor/autoload.php
```

The name resolves against the application root. The runtime skips a missing file. The setting names what to load when the tree holds the file. One configuration then covers a checkout without `vendor/`.

One setting covers every way the tree runs. The server includes it before each request, `phpscript run` before the script, `phpscript test` before each fixture and `phpscript lint` before the checks. The classes a linter knows are then the classes a request will find. `--include` on any command overrides this setting. A virtual host sets `include` in the `runner` block of its `phpscript.yml`.

Classes still resolve through `spl_autoload_register()`, the function a composer autoloader registers with. See [Autoloading](reference/namespaces/README.md#autoloading) in the language reference.

### Writable paths

`writable_paths` names the directories a script may write to. Entries are relative to the application root. A site accepting uploads writes:

```yaml
runner:
  writable_paths: ["upload", "public/upload"]
```

`upload` is the project's own directory, reachable by scripts alone. `public/upload` is below the document root. The server publishes a script's stored files there over HTTP at `/upload/...`, including files written while the server runs.

No entry grants a write outside the application root. The allowlist accepts an absolute entry and then never matches it. All paths a script names resolve against the root first, absolute ones included. A script writing `/srv/shared/x` writes to `/srv/shared/x` *below the root*, not inside the `/srv/shared` the entry named. A host that must write elsewhere binds a different root.

An empty list, the default, allows every write. A non-empty one is a tree: the directory and everything below. A name sharing only the prefix does not qualify. `upload-old` is then not inside `upload`.

Writes refused by the allowlist **throw**. A write the operating system refused returns `false`, as PHP's does. A script continuing as though a write happened is the failure worth stopping. The exception is catchable:

```php
try {
    move_uploaded_file($file["tmp_name"], "public/upload/" . $name);
} catch (Exception $e) {
    echo "refused: " . $e;
}
```

The functions held to it are the ones that change the filesystem: `fopen()` in any mode but `r`, `mkdir()`, `unlink()`, `touch()`, `rename()` at both ends, `copy()` at the destination, `chmod()`, `chown()`, `chgrp()` and `move_uploaded_file()`. Reads go untouched: an allowlist names the writes a script may make.

Configuring it also changes how the server treats those directories:

- **The server runs no PHP in a writable directory.** It serves a `.php` file there as bytes like any other file. A directory a visitor can get content into is not a directory to run code from.
- **The route scan skips an `@route` or `@startup` annotation in one.** Otherwise an uploaded file could publish a route the next time the server started.

### Execution limits

The runtime enforces `memory_limit`. It measures usage by walking the live variables of every execution frame. The number counts the values the script still holds. It does not count the allocation total. The walk runs when the script calls `memory_get_usage()` and at periodic checkpoints while a limit applies. Exceeding the limit raises a `RuntimeException` the script may catch. The number is an estimate of PHP value payloads, not Go allocator truth, and falls far below what PHP reports for the same script (no zval overhead).

What outlives a request is outside the usage number and outside the limit: the parsed and compiled source tree, plus a `SharedMemory` store a host bound. Every request the process serves shares both and charging either to whichever script asked would report one process-lifetime cost once per request. PHP draws the same line, charging a compiled script to `opcache.memory_consumption` and not to `memory_limit`.

`phpinfo()` shows that side. It prints the runtime block first, then a section per subsystem holding memory across requests:

```
OPcache

Precompile => true
Cached Files => 128
Compiled Expressions => 3471
Cached Size => 4.20 MiB

SharedMemory

Entries => 12
Counters => 3
Size => 0.06 MiB
```

`Cached Size` is the heap a precompile pass measured itself adding. The value is absent when no pass runs: the runtime never measures a cache filled request by request. Printing zero would read as a measurement. A section with nothing to report drops out, not printed as a heading over nothing. A run without a store bound shows no `SharedMemory` block. The `opcache_*` functions are not implemented, and will not be. See [Won't build](design.md#wont-build).

phpscript **accepts `time_limit` and `concurrency_limit` without enforcing them**. Both keys parse and travel through to the runtime so a configuration written today keeps working when enforcement lands.

`memory_limit` is a size written the way the upload limits are. `time_limit` is php.ini's `max_execution_time`, in seconds. `concurrency_limit` has no php.ini counterpart: the SAPI owns that number. Here one process serves more than one site, each with its share.

### Sizes

`max_input_vars` and `max_input_nesting_level` are php.ini's, and bound the structure the form decoder builds from an attacker-controlled body: `a[x][x][x]...` costs one array per level. See [`$_GET`](reference/predefined-variables/README.md#_get).

`upload_max_filesize` and `post_max_size` are php.ini's, with php.ini's defaults. Write a size as a bare number of bytes or as a number with an `M` suffix for megabytes. phpscript rejects php.ini's `K` and `G` shorthands. A value that does not parse fails the configuration file. `0` means no limit.

```yaml
runner:
  upload_max_filesize: 20M
  post_max_size: 25165824
```

### Upload file mode

An upload arrives in a temporary file readable only by the process that wrote it. `move_uploaded_file()` applies a mode when it stores one. `upload_file_mode` is that mode, written in octal the way `chmod()` takes it, with or without the leading zero. PHP has no setting for this, it leaves the mode to the process umask. Naming it here means an upload lands the same way whatever the umask of the process is.

```yaml
runner:
  upload_file_mode: "0640"  # owner writes, group reads, nobody else
```

The default, `0644`, is the mode a umask of 022 produces. A web server in front of the application can read it. Services that serve uploads themselves, or pass them to a worker in the same group, have no reason to make them world readable: `0640`, or `0600` for files only this process should open.

A request over either size limit is not something a script can catch: it happened before the script started. A script sees only an empty superglobal or an `UPLOAD_ERR_INI_SIZE` entry in `$_FILES`. The runtime reports the reason to the Go host through `Runtime.RecordError`, which puts it on the request trace and passes it to a handler installed with `Runtime.OnError`. See [Errors](reference/errors/README.md#errors-a-script-cannot-catch) and [`$_FILES`](reference/predefined-variables/README.md#_files).

## Runtime backend

Set `flatstack.enabled` to `true` to select the experimental flat-stack bytecode backend. Programs outside its native subset transparently use the compatible runner implementation. See [Flat-stack runtime](./flatstack.md) for the current native subset and fallback behavior.

## Server

`server` configures the [platform](https://github.com/titpetric/platform) that `phpscript server` runs on. It applies to that command only.

| Key        | Default | Purpose                                                             |
|------------|--------:|---------------------------------------------------------------------|
| `addr`     | `:8080` | Address the HTTP server listens on.                                 |
| `quiet`    | `false` | Turn down platform lifecycle logging.                               |
| `modules`  |    `[]` | Load only the platform modules named here. An empty list loads all. |
| `pid_file` |    `""` | File the server records its process id in. Empty writes none.       |

`pid_file` makes a running server reachable. [`phpscript -s reload`](cli/server.md#reloading) reads the pid back out of it. `kill -HUP $(cat ...)` reads the same pid. An empty path writes no file. A reload then has no pid to read. The file holds the decimal pid, then a newline. A clean shutdown removes the file. A file left behind means the process did not get a pid. The path resolves against the working directory. `-s reload` then needs the same `-w` as the running server. A service unit names an absolute path.

The process reads every key here once, at startup. A reload re-reads the file and cannot apply any key. The reload reports which keys it skipped. See [Reloading](cli/server.md#reloading).

This section, together with `telemetry` below, is the only source of the platform's options. phpscript builds them from the configuration file and the platform's `PLATFORM_SERVER_ADDR`, `PLATFORM_MODULES` and `PLATFORM_TELEMETRY_*` environment variables are not read. The `PLATFORM_DB_*` variables fall outside the platform's options and phpscript still reads them. See [Database connections](#database-connections).

## HTTP modules

`routes.enabled` controls whether `phpscript server` recursively scans PHP files outside `public/` for `// @route` annotations. Static files and directly requested PHP entrypoints under `public/` are independent of this setting.

## Document root

`document_root` is the directory beneath the application root served over HTTP by `phpscript server`. The default is `public`. An application has no reason to change that. The key exists for a tree that already calls that directory something else.

```yaml
document_root: web
```

The server runs a `.php` file found there, serves anything else as a static file, and answers a request that names a directory with that directory's `index.php`, or its `index.html` when `index.php` is missing. The entrypoint's name resolves against the application root. The document root does not affect it. Any `include` reaching above the served directory still resolves inside the project. Route scanning also skips the directory. An annotation in a file the server serves directly does not publish another unguarded route.

The directory also holds a site's error pages. A file named after a status answers for it: `404.php`, `503.html`, or `error.php` for the statuses a site does not name. Writing the file is the whole of turning it on. That is a convention. No key here turns it on. See [Error pages](use-cases/error-handling.md#error-pages) for the data a page receives and the requests that get one.

## Autoindex

`autoindex` answers a directory that has no index page with a listing of its contents, as nginx's `autoindex on;` does. The default is off:

```yaml
autoindex: true
```

With autoindex disabled, a directory holding neither `index.php` nor `index.html` is a 404, and the site's [error page](use-cases/error-handling.md#error-pages) answers it like any other dead link. Publishing every file below the document root is a site's decision, not a default it should reach by leaving an `index.html` out.

The server generates the listing. No file needs writing or keeping in step with the directory. It names each entry, its size and when it last changed, renders images inline, and links to the directory above. The listing drops names beginning with a dot. The page styles itself. It fetches nothing from anywhere else. A listing does not tell a third party the pages a visitor browses.

Virtual hosts each answer for themselves: each site reads the key from its `phpscript.yml`. One domain can publish a file drop while another does not. See [Serving static files](use-cases/static-files.md).

## Telemetry

`telemetry.enabled` controls request tracing and the debug front end mounted at `telemetry.path`. The section is [oida](https://github.com/titpetric/oida) options and it takes every field that library documents, plus `driver` and `storage_path`, which phpscript adds.

The platform owns the only recorder: given this section it builds the tracer, wraps every module it runs in the tracing middleware and mounts the front end. That is why this block sits outside `server` even though the platform consumes it. It also applies to `phpscript server` alone. phpscript registers no other recorder. It reports interpreter work, the includes, calls and templates of a request and the spans a script starts itself, onto the trace that middleware already started. That work shows up on the same front end as the request that caused it.

The fields that matter for a phpscript service are:

| Key                   | Default                           | Purpose                                                                   |
|-----------------------|----------------------------------:|---------------------------------------------------------------------------|
| `enabled`             |                            `true` | Record traces. When false the middleware passes requests through.         |
| `path`                |                     `/debug/oida` | Mount path of the debug front end.                                        |
| `service_name`        |                       `phpscript` | Name shown in the front end and recorded on every trace.                  |
| `ring_buffer_size`    |                             `200` | Number of completed traces retained for the list and rolling statistics.  |
| `top_requests`        |                              `20` | Upper bound on trace groups returned by rolling statistics.               |
| `max_spans_per_trace` |                            `1000` | Spans recorded per trace; the recorder counts and drops further spans.    |
| `sample_rate`         |                             `100` | Percentage of requests traced, `0` to `100`.                              |
| `track_memory_use`    |                            `true` | Record process-wide allocation and garbage-collection changes per trace.  |
| `live_stream`         |                            `true` | Serve the live view over server sent events instead of a refresh timer.   |
| `ignore_paths`        |                  health endpoints | Paths never traced. Entries ending in `/*` match by prefix.               |
| `driver`              |                          `memory` | Trace store: `memory` (ring buffer) or `disk` (JSON files, restart-safe). |
| `storage_path`        | `/dev/shm/phpscript-trace-detail` | Folder for `driver: disk`. One `{id}.json` per retained trace.            |

Memory tracking has process-wide sampling overhead and concurrent requests can overlap in those measurements. See [Telemetry](./telemetry.md) for the views, the representations and the records PHP can make.

A server running [virtual hosts](#virtual-hosts) is the exception: each site builds a tracer from its `telemetry` block and mounts a separate front end. The operator then turns that block off.

## Database connections

Each `env` item with a `PLATFORM_DB_<NAME>=<driver>://<dsn>` key registers a named connection for `Database`. phpscript lowercases the names and this configuration:

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

phpscript reads the list into the database connection registry and does not add the entries to the process environment or PHP variables. Actual process environment variables named `PLATFORM_DB_*` are also registered when phpscript starts.

In a virtual host, a relative sqlite path resolves against the site's application root and `sqlite://data/app.db` in a site's `phpscript.yml` names the `data/` directory of that site whatever directory the server runs in. phpscript takes absolute paths, memory databases and `file:` URIs as written. Outside virtual hosts, a CLI run or a single application root, a relative path resolves against the process working directory.

## Mail servers

`mail` is a map of named mail servers. A script names a server. The key is the name:

```yaml
mail:
  default:
    host: mail.example.com
    port: 587
    username: noreply@example.com
    password: secret
    from: Example <noreply@example.com>
  marketing:
    host: relay.example.net
    username: campaigns
    password: another-secret
    from: Marketing <marketing@example.com>
```

is available to PHP as:

```php
$mail = new Mail;              // the "default" server
$campaigns = new Mail("marketing");
$campaigns->send("hello@example.com", "Subject", "Body");

mail("hello@example.com", "Subject", "Body");  // also the "default" server
```

| Key        | Default  | Purpose                                                                                                    |
|------------|---------:|------------------------------------------------------------------------------------------------------------|
| `host`     | required | The mail server to reach.                                                                                  |
| `port`     |     `25` | The port to reach it on. 587 is the submission port.                                                       |
| `username` |       "" | Sent with `password` as PLAIN authentication. Neither set means no authentication, which mailhog accepts.  |
| `password` |       "" | See `username`.                                                                                            |
| `from`     | required | The `From` header. It may hold a display name, in which case the bare address becomes the envelope sender. |
| `insecure` |  `false` | Accept the STARTTLS certificate without verifying its chain or names. See below.                           |

phpscript compares names lowercased. The constructor refuses a server the configuration did not name, before a script has composed a message:

```text
no configuration found for mail server: transactional
```

Without a configured server, `mail()` and `new Mail` still exist and fail the same catchable way. Naming `default` keeps one spelling and its fallback.

The client upgrades the connection with STARTTLS whenever the server advertises it. `insecure: true` accepts the certificate without verifying its chain or names, as a host with a self-signed certificate requires, or a certificate without a `subjectAltName`. The session stays encrypted and open to a man in the middle. Prefer a certificate the host can verify.

### Credentials stay with the host

A script names a server. It cannot spell one or read one back:

- The constructor takes a name. No binding accepts a host or a password.
- The object it returns holds no properties. `$mail->password`, `get_object_vars($mail)`, `var_dump`, `print_r` and `json_encode` all find nothing.
- No listing call exists. `Mail` has no counterpart to [`Database::connections()`](#database-connections), because naming what exists is itself a disclosure.
- The block is configuration. No part of `env` and `getenv()` can reach it.

This is why the constructor takes no settings argument. phpscript refuses a script that passes them:

```text
Mail(): argument #1 ($name) must be the name of a configured server, not the settings of one: connection settings are the host's, and belong in the mail block of its configuration
```

A flat block, the single unnamed server phpscript accepted before the key became a map, fails the command naming what to write instead:

```text
config.yml: mail.host is not a server name: mail is a map of named servers and a flat block moves under a name such as "default"
```

A server missing `host`, missing `from`, or naming an invalid `from` fails the command at startup, not on the first delivery. On a site whose only sender is a `@schedule` job, that failure lands at night on the path nobody watches.

## Virtual hosts

`virtualhost` lists the sites one `phpscript server` answers for, one entry per site. While the list is empty the server serves a single application root, named on the command line. With entries present, the `Host` header selects the site. An application root on the command line is then an error: the entries name their own roots. A further root belongs to no site.

| Key             | Default  | Purpose                                                                                     |
|-----------------|---------:|---------------------------------------------------------------------------------------------|
| `domain`        | required | The `Host` headers this entry answers for, separated by spaces.                             |
| `root`          | required | The application root, the directory holding the site's `phpscript.yml`.                     |
| `document_root` | `public` | Directory beneath `root` served over HTTP. It wins over the site's own `document_root` key. |

A site answering to more than one name lists them all in `domain`. They are names for one site, not copies: the server builds the site once. Every name shares its routes, its recorder and its connections:

```yaml
virtualhost:
  - domain: example.com www.example.com
    root: /srv/example
```

The first name is the site's own: errors report the site as that name, and the names of the modules it registers end in it.

Listing the same name twice, in one entry or in two entries, is a startup error: the second would silently win.

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

The entry says which domain reaches a site and where it lives. Everything else about the site comes from its file. [Virtual hosting](use-cases/virtual-hosting.md) works a two site server through in full.

### The site's configuration file

Each `root` must hold a `phpscript.yml`. The server reads it on top of the configuration passed with `-f` the same way it reads that file on top of the embedded defaults and the site file names only the keys it changes and inherits the rest. A `root` without a site file fails startup. The server then serves no site under settings its author never wrote.

A site may not set `server` or `virtualhost`: the listen address belongs to the operator. No site holds sites. Either key stops startup. A site author is then never left believing they moved the listen address:

```text
virtualhost "shop.example.com": /srv/shop/phpscript.yml: "server" is set by the operator, not by the site
```

### Host matching

The server lowercases a `Host` header, strips its port and any trailing dot, and compares the result against domains normalized the same way. `shop.example.com`, `SHOP.Example.com.` and `shop.example.com:8080` all reach the same site. Wildcards do not exist. No entry is a default site. A `Host` no entry claims gets 404, never reaching a site's code.

Each site is a separate router. Its `@route` endpoints and its document root answer on its domain, nowhere else. The site's `@startup` and `@schedule` jobs run on its behalf. The platform modules those jobs run as take a per-site name: `phpstartup:shop.example.com` and `phpschedule:shop.example.com`. `server.modules` can still name one site's modules.

### Telemetry per site

A site's `telemetry` block builds a tracer and mounts a debug front end inside the site's router. The dashboard and its traces belong to that domain alone.

Turn the operator's own `telemetry` off when running virtual hosts. The platform mounts its dashboard on the root router, in front of the host mux. That path prefix answers on every domain, including a domain no entry claims, and shadows the front end each site mounts there. Startup says so when a site names the operator's path explicitly:

```text
virtualhost "shop.example.com": telemetry path "/debug/oida" is the path the server mounts its own dashboard on
```

The check only fires for a path the site's file names itself. It skips a site that names none. Turning the operator's block off makes the site dashboards reachable.

Two sites running `driver: disk` may not share a `storage_path`, or their traces would land in one store.

### Databases per site

A site builds its connections from its `env`. The provider holding them sees no other site's connection. A site can open only the connections its file names. `new Database("shop")` on a site without `shop` configured fails with `no configuration found for database: [shop]`.

`env` is a list, and the overlay replaces a list wholesale. A site that declares an `env` gets only its connections, while a site that declares none inherits every connection the operator configured. The shipped `config/config.yml` holds one `PLATFORM_DB_*` entry and setting `env: []` in the operator's file is the way to run virtual hosts without shared connections.

### Mail servers per site

A site's mail servers come from its `mail` block. Its provider holds only that set. A site can name only the servers its file configured. `new Mail("marketing")` on a site without `marketing` configured fails with `no configuration found for mail server: marketing`.

The map replaces wholesale. A site that declares any server gets only its servers, while a site that declares none inherits the operator's:

```yaml
mail:
  marketing:
    host: relay.example.net
    from: marketing@shop.example.com
```

A site writing that has `marketing` but no `default`, even though the operator configured one.

`mail:` with nothing beneath means no servers at all, the inherited ones included. A site writes that to say it sends no mail.

This replacing is why the block is a map. A single unnamed server is a struct, which merges field by field: a site setting only `host` and `from` would keep the operator's `username` and `password`, then go on authenticating as the operator.

### The environment per site

`env` is also the site's only script environment, read through `getenv()`. A site reads only the environment it declared. `getenv()` cannot report the declarations of the operator, or of another site.

Variables named `PLATFORM_*` are the exception in the other direction. They configure phpscript and the platform it runs on, connection strings included and they are never visible to a script even when the site declared them itself. A site that lists `PLATFORM_DB_SHOP` gets the connection and reads nothing from `getenv("PLATFORM_DB_SHOP")`.

A single application server, without `virtualhost` entries, keeps the process environment plus whatever `env` adds, minus the same `PLATFORM_*` variables.

### Startup checks

phpscript loads and checks the whole list before the server listens. A broken entry fails startup before any request arrives:

| Check                                                      | Failure |
|------------------------------------------------------------|---------|
| Every entry names a `domain` and a `root`                  | error   |
| No host appears twice, in one `domain` or across two       | error   |
| `root` exists and is a directory                           | error   |
| `root/phpscript.yml` exists and parses                     | error   |
| The site does not set `server` or `virtualhost`            | error   |
| The document root exists beneath `root`                    | error   |
| Two `driver: disk` sites do not share a `storage_path`     | error   |
| A site does not claim the operator's telemetry path        | error   |
| An application root is not also passed on the command line | error   |

`phpscript -t` runs every check without starting anything and an operator can check a configuration before the server serves it or a reload applies it. For a single application root, which has no entries to check, it checks that the root and its document root exist. See [Testing a configuration](cli/server.md#testing-a-configuration).

### Startup jobs per site

A site's `@startup` jobs belong to it. That site's recorder takes a failing job as a background trace. The server keeps serving: the alternative lets one site's broken job stop the other sites in the process. The remaining jobs of that site still run: they are independent. The run reports every failure, the first included.

A single application server, without `virtualhost` entries, keeps a failing `@startup` fatal. There is no other tenant to protect. A process that came up with its schema unapplied is worse than a process which never came up.

## Test suites

`test` configures [`phpscript test`](cli/test.md). It holds a folder's declaration about its fixtures. A suite needing a bootstrap or a database keeps that in a file, off every command line that reaches it.

A `phpscript.yml` found in the fixture tree marks a **suite root**, the directory governing the fixtures below. Each fixture resolves the nearest such file at or above its directory. The run resolves the nearest one at or above the working directory. A tree holding none behaves as it always did.

| Key              | Default  | Scope      | Purpose                                                    |
|------------------|---------:|------------|------------------------------------------------------------|
| `include`        |     none | suite root | File included ahead of every fixture below the suite root. |
| `hooks.setup`    |     none | suite root | File run once before those fixtures.                       |
| `hooks.teardown` |     none | suite root | File run once after them.                                  |
| `parallel`       |      `1` | run        | Fixtures of one area running at once, `--parallel`.        |
| `cache`          | `worker` | run        | How far a parsed include travels, `--cache`.               |
| `skip_php`       |  `false` | run        | Leave the `php` binary out of a matrix run, `--skip-php`.  |

The flag wins over the file. A configuration describes a tree. A flag records the operator's intent for this run. `-p 1` on a file naming four sets the run to one at a time.

### The suite root

`include` and the hook files resolve against the directory holding the file, which is also the application root of the fixtures below. Their own folder still answers first. Its relative includes keep their meaning. The suite root supplies the files the folder does not hold.

`--include` is the operator's. It speaks from the directory holding the command, not from a suite. It replaces what any suite named.

### Run keys

`parallel`, `cache` and `skip_php` describe one run of the whole command. A `phpscript.yml` discovered below the invocation root may not set them. Either is a startup error naming the key. No value is silently dropped:

```text
tests/integration/phpscript.yml: "test.parallel" is set by the run, not by a suite
```

The run's own file does set them. That is the file `-f` named, or the nearest `phpscript.yml` at or above the working directory. A suite file may not name `server` or `virtualhost`, for the reasons a site's file may not: a fixture run has no listen address and holds no sites.

### Hooks

Both hooks run once per session, not once per fixture. Both are ordinary PHP with the bindings a fixture below them gets. `setup` lays down the schema its fixtures query and the rows they read:

```yaml
env:
  - "PLATFORM_DB_SCAFFOLD=sqlite://file:phpscript-scaffold?mode=memory&cache=shared"

test:
  hooks:
    setup: setup.php
```

```php
<?php

$migrate = new Database\Migrate("scaffold");
$migrate->load("./schema/*.up.sql");
$migrate->run();

$db = new Database("scaffold");
$db->insert("catalogue", array("name" => "Ada"));
```

A failing `setup` fails the run before a fixture executes. The fixtures below assert against state nothing laid down. Running them reports the same missing table once per file, without naming the cause.

`teardown` runs whether the fixtures passed or failed. The run reports its failure without displacing theirs. Where one run covers more than one suite, the setups run outermost first and the teardowns in reverse.

Hook output reaches the terminal only under `-v`. Without the flag a run answers with a folder table. A schema's log lines in the middle are noise.

These are configuration keys, separate from the `@startup` annotation. Annotations are server surface: `phpscript server` scans `@route`, `@startup` and `@schedule` out of a source tree and runs them per virtual host or per application root depending on its configuration. A fixture run is neither of those scopes, and nothing holds a suite to a comment in a file that a walk happened to reach. The two also do different work: a server's `@startup` applies the schema an application boots with, where a suite's `setup` also seeds the rows its fixtures assert against.

### Databases per suite

A suite's `env` builds the connections its fixtures resolve, the same way a virtual host's does. The list replaces wholesale. A folder that names its connections gets only those:

```php
$db = new Database("scaffold");     // the folder configured it
new Database("sqlite_test");        // no configuration found for database: [sqlite_test]
```

A suite that names no `env` inherits the run's environment, which for a CLI run is the process environment. A folder that configured no connections gets no separate set.

The setup hook and the fixtures below share one connection pool. Two pools on one DSN are two databases whenever the DSN names no shared file. A schema applied through the first is absent from the database the fixtures query.

### Mail servers per suite

A suite's `mail` block names the servers its fixtures may open, by the same replacing rule. A fixture cannot name a server its folder did not configure. `tests/fixtures/mail` is the worked example.

A fixture run delivers nothing. Constructing a `Mail` is a name lookup and dials nothing. Only `send()` reaches a server. A fixture run has no server to dial. A fixture about resolution reads only the names from the block.
