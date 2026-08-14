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

flatstack:
  enabled: false

routes:
  enabled: true

status:
  enabled: true
  options:
    ring_buffer_size: 100
    top_requests: 20
    track_memory_use: true

env:
  - "PLATFORM_DB_APP=sqlite://app.db"
```

The external file replaces the embedded YAML as the configuration source.
Routes and status remain enabled when their sections are omitted because those
are model defaults; other omitted values use their Go zero values. Start with
the complete example when you want behavior to be explicit.

## Runner

`runner` applies to the `run` and `server` commands and to annotated routes
created by the bundled server.

| Key | Default | Purpose |
|---|---:|---|
| `work_dir` | `.` | Directory inside the runtime source filesystem used to resolve relative script and include paths. |
| `writable_paths` | `[]` | Writable-path allowlist exposed to filesystem integrations. An empty list adds no allowlist restriction. |

## Runtime backend

Set `flatstack.enabled` to `true` to select the experimental flat-stack
bytecode backend. Programs outside its native subset transparently use the
compatible runner implementation. See [Flat-stack runtime](./flatstack.md) for
the current native subset and fallback behavior.

## HTTP modules

`routes.enabled` controls whether `phpscript server` recursively scans PHP files
outside `public/` for `// @route` annotations. Static files and directly
requested PHP entrypoints under `public/` are independent of this setting.

`status.enabled` controls the server status module and its
`/debug/server-status` endpoints. Its options are:

| Key | Default | Purpose |
|---|---:|---|
| `ring_buffer_size` | `100` | Number of completed requests retained for the log and rolling statistics. |
| `top_requests` | `20` | Maximum request groups returned by rolling statistics. |
| `track_memory_use` | `true` | Record process-wide allocation and garbage-collection changes per request. |

Memory tracking has process-wide sampling overhead and concurrent requests can
overlap in those measurements. See [Server status middleware](./server-status.md)
for endpoints, representations, and interpretation.

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
