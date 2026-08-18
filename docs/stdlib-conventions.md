# Standard library design conventions

The project standard library is smaller than PHP's and smaller than Go's.
Bindings are added because a script needs them, not to complete a catalogue.
This page is the mapping rule for new work: pick a namespace from the table,
do not invent a third spelling for the same idea.

## Two homes

1. **PHP-compatible names** when the call is already a PHP function and the
   behaviour can stay close enough. `json_encode`, `preg_match`, `explode`
   stay global functions. A caller who already knows PHP should not relearn
   them.
2. **A short namespace** when the API is ours, or when PHP has no honest
   equivalent. Host-backed objects live here. The historical nest is `PS\`;
   new unique APIs use a single top-level segment that matches the Go package
   people already import.

`PS\` remains valid for what is already registered. New unique APIs do not
add a second prefix on top of it (`PS\HTTP\Client` is rejected).

## Mapping from Go packages

The PHP namespace is the last path segment of the Go import, uppercased, unless
the table below says otherwise. `net/http` becomes `HTTP`, `database/sql`
becomes `SQL`, `encoding/json` stays the PHP functions.

| Go package | PHP today | Namespace | Status |
|---|---|---|---|
| (project helpers) | `PS\SharedMemory`, sessions | `PS\` | Existing. Do not grow it. |
| `database/sql`, `titpetric/pdo` | `Database`, `Database\Migrate` | `SQL\DB`, `SQL\Migrate` as the long-term spelling; `Database` stays as the alias already bound | Bound |
| `net/http` | — | `HTTP\Client`, `HTTP\Request`, `HTTP\Response`, `HTTP\Server` | Unimplemented |
| `encoding/json` | `json_encode`, `json_decode` | PHP functions | Bound |
| `goccy/go-yaml` | — | `yaml_encode`, `yaml_decode` (PHP function shape) | Unimplemented |
| OpenTelemetry / oida | `start_span`, `/debug/oida` | `Telemetry` (no `PS\` prefix). The Go package is `telemetry/`. | Partial |
| `net/smtp` / project SMTP | `mail`, `SMTP` | `SMTP` | Bound |
| `titpetric/minitpl` | native templates | PHP-compatible | Bound via composer |

## How to name a new binding

1. If PHP already has the function and we can honour the common case, register
   that function name. Do not invent `JSON\Encode`.
2. If the unit of work is an object with methods (a client, a connection, a
   span), register a constructor under the namespace from the table.
   `new HTTP\Client()` mirrors `http.Client`. `new SQL\DB($name)` mirrors
   `sql.Open`.
3. Methods use the PHP spelling of the Go method: `begin()`, `query()`,
   `set_attribute()`. One word, snake or short camel as the existing class
   already does; do not mix both on one type.
4. Errors return as thrown values, the same way a Go `(T, error)` constructor
   already surfaces today.

## Scope of interest

These are the only areas expected to grow:

- **HTTP** — client and, later, a small server helper. Not a curl clone.
- **SQL** — the existing `Database` surface, documented as `SQL\DB` when a
  rename is cheap. No second client.
- **Encoding** — JSON stays PHP. YAML follows JSON.
- **Telemetry** — spans and the oida dashboard. Not an OpenTelemetry SDK.
- **Mail** — the existing `SMTP` / `mail()` pair.
- **Templates** — minitpl, not a new engine.

Anything outside that list needs its own issue before a namespace is minted.

## What this is not

- Full PHP extension coverage.
- A Go standard-library mirror (`os`, `io`, `sync` stay in Go).
- A place to park one-off helpers under `PS\` because the name is free.
