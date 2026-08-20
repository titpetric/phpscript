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
  upload_max_filesize: 2M
  post_max_size: 8M
  upload_file_mode: "0644"

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

| Key                   | Default | Purpose                                                                                                  |
|-----------------------|--------:|----------------------------------------------------------------------------------------------------------|
| `work_dir`            |     `.` | Directory inside the runtime source filesystem used to resolve relative script and include paths.        |
| `writable_paths`      |    `[]` | Writable-path allowlist exposed to filesystem integrations. An empty list adds no allowlist restriction. |
| `upload_max_filesize` |    `2M` | Largest file part a request may carry. A part over it is refused and reported in `$_FILES`.              |
| `post_max_size`       |    `8M` | Largest request body that is parsed at all. A body over it leaves `$_POST` and `$_FILES` empty.          |
| `upload_file_mode`    |  `0644` | Mode `move_uploaded_file()` gives a stored upload. Octal, as `chmod()` takes it.                         |

### Sizes

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
