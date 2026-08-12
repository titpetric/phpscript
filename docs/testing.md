# Testing phpscript

Run the full test suite from the repository root:

```bash
go test ./...
```

Most language and runtime behavior should be covered by a `.phpt` fixture.
Use a Go test in the package that owns the behavior when the assertion needs
direct access to Go APIs, parser models, runtime state, concurrency, or error
types.

## Go test utilities

The [`tests` package](../tests) provides reusable bindings and setup for tests
that need to exercise the runtime from another Go package. Import it with an
alias to make clear that these are repository test helpers, not production
APIs:

```go
import testutil "github.com/titpetric/phpscript/tests"
```

### Database setup

Importing `tests` registers the MySQL, PostgreSQL, and SQLite
`database/sql` drivers. An external test package can also delegate its
`TestMain` to `tests.TestMain`:

```go
func TestMain(m *testing.M) {
	testutil.TestMain(m)
}
```

The helper sets (and overwrites) the database DSNs used by the integration
fixtures before running the suite:

| Environment variable | Configured test service |
|---|---|
| `DB_DSN_SQLITE_TEST` | Shared in-memory SQLite database |
| `DB_DSN_POSTGRES_TEST` | PostgreSQL on `localhost:15432` |
| `DB_DSN_MYSQL_TEST` | MySQL on `localhost:13306` |

The PostgreSQL and MySQL values correspond to the services in
[`compose.yml`](../compose.yml). `TestMain` calls `os.Exit`, so call it as the
external package's complete `TestMain` rather than from an individual test.

### Storage binding

[`NewStorage`](../tests/storage.go) constructs an isolated in-memory
[`Storage`](../tests/storage.go) binding. Registering it as a constructor tests
the complete Go-to-PHP bridge: automatic `context.Context` injection, methods,
rich [`Record`](../tests/storage.go) return values, slices of records, and
returned errors becoming PHP exceptions.

```go
rt.RegisterConstructor("Storage", testutil.NewStorage)
rt.RegisterConstructor("FailStorage", testutil.NewFailStorage)
```

PHP can then create and use the Go value:

```php
$storage = new Storage;
$storage->set("color", "blue");
$record = $storage->get("color");
echo $record->key . "=" . $record->value;
```

Each `new Storage` gets a new map. `get()` returns an error for a missing key,
`all()` returns records sorted by key for deterministic assertions, and `len()`
reports the number of records. `NewFailStorage` always returns `boom` for
constructor-error tests. The fixture harness additionally injects the tenant
`acme`, which `tenant()` exposes.

### Shared-memory binding

The production [`ps.SharedMemory`](../stdlib/ps/shared_memory.go) binding is
also the test utility for state shared by otherwise independent runtimes,
especially route requests. Create one value, place it in each runtime context,
and register the standard library:

```go
shm := ps.NewSharedMemory()

rt.SetContext(ps.SharedMemoryContext(rt.Context(), shm))
stdlib.Register(rt)
```

PHP accesses this constructor as `PS\SharedMemory`. The binding exposes `set`
and `get` for string values, `incr` and `count` for named counters, and
`delete`, `has`, and `clear` for cleanup and assertions. Reuse the same `shm`
across runtimes to assert cross-request state; create a new one per test to
avoid state leaking between tests.

Tests for a custom, unnamespaced PHP API can register the same production
constructor under another name:

```go
rt.RegisterConstructor("SharedMemory", ps.NewSharedMemoryBinding)
```

## `.phpt` fixtures

Fixtures live directly in [`tests/fixtures`](../tests/fixtures). The test
harness discovers every file with a `.phpt` extension and runs it through the
default `runner` runtime.

Each fixture has three sections separated by a line containing only `---`:

```text
<YAML metadata>
---
<PHP source>
---
<expected output>
```

For example:

```phpt
name: string concatenation
description: Concatenating two strings produces their combined value.
---
<?php
$greeting = "hello";
echo $greeting . " world";
?>
---
hello world
```

The YAML metadata supports these fields:

| Field | Required | Purpose |
|---|---:|---|
| `name` | yes | Human-readable subtest name. |
| `description` | yes | Behavior and intent covered by the fixture. |
| `error` | no | Substring that must occur in the chain of an uncaught runtime error. |
| `stdin` | no | String exposed to the script through `STDIN`. |
| `flatstack` | no | Set to `true` to run the fixture through flatstack as well. |

The expected-output section is always checked. Trailing newline differences are
ignored, but all other output must match exactly. For an uncaught error, set
`error` to a stable identifying substring and normally expect the host response
body `Internal Server Error`:

```phpt
name: uncaught exception
description: An uncaught exception is returned to the embedding host.
error: boom
---
<?php
throw new Exception("boom");
?>
---
Internal Server Error
```

Files used by `include`, autoloading, templates, or filesystem APIs can be
placed below `tests/fixtures`. The fixture runtime exposes that directory as
its root filesystem, so fixture code refers to those files with paths relative
to it.

## Testing with flatstack

Every `.phpt` fixture runs through the default runtime. To additionally run a
fixture through the native flatstack bytecode runtime, add one metadata field:

```yaml
flatstack: true
```

No Go-side fixture list needs updating. The flatstack harness discovers the
flag automatically. It also calls `flatstack.Supports` before execution, so the
test fails if the program would use the compatibility interpreter instead of
native flat bytecode. Only add the flag after all syntax and runtime operations
used by the fixture are supported by flatstack.

## Running fixture tests

Run all default-runtime fixtures:

```bash
go test ./tests -run '^TestFixtures$'
```

Run all flatstack-enabled fixtures:

```bash
go test ./tests -run '^TestFlatstackFixtures$'
```

Use the fixture name to focus on one subtest:

```bash
go test ./tests -run 'TestFixtures/string_concatenation'
go test ./tests -run 'TestFlatstackFixtures/string_concatenation'
```

Before submitting a change, run the package tests affected by the change and
then the complete suite with `go test ./...`.
