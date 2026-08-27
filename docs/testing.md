# Testing phpscript

Run the full test suite from the repository root:

```bash
go test ./...
```

A change to language or runtime behavior lands with a `.phpt` fixture. Use a Go test in the package that owns the behavior when the assertion needs direct access to Go APIs, parser models, runtime state, concurrency, or error types, and add the fixture as well: a Go test proves the Go code does what its author meant, a fixture proves a script sees it.

A fixture does two jobs, and both are required of it:

1. It states the behavior, so a change that alters the behavior fails a named test instead of surfacing in a benchmark or a demo.
2. It records what PHP itself produces, so the runtime is measured against the language rather than against its own previous output.

The second job is the one that finds defects. Expected output written from what phpscript currently prints locks in whatever it prints, including the parts that are wrong. Expected output taken from `php` makes the fixture a compatibility check.

## Go test utilities

The [`tests` package](../tests) provides reusable bindings and setup for tests that need to exercise the runtime from another Go package. Import it with an alias to make clear that these are repository test helpers, not production APIs:

```go
import testutil "github.com/titpetric/phpscript/tests"
```

### Database setup

Importing `tests` registers the MySQL, PostgreSQL, and SQLite `database/sql` drivers. An external test package can also delegate its `TestMain` to `tests.TestMain`:

```go
func TestMain(m *testing.M) {
	testutil.TestMain(m)
}
```

The helper sets (and overwrites) the database DSNs used by the integration fixtures before running the suite:

| Environment variable   | Configured test service          |
|------------------------|----------------------------------|
| `DB_DSN_SQLITE_TEST`   | Shared in-memory SQLite database |
| `DB_DSN_POSTGRES_TEST` | PostgreSQL on `localhost:15432`  |
| `DB_DSN_MYSQL_TEST`    | MySQL on `localhost:13306`       |

The PostgreSQL and MySQL values correspond to the services in [`compose.yml`](../compose.yml). `TestMain` calls `os.Exit`, so call it as the external package's complete `TestMain` rather than from an individual test.

### Storage binding

[`NewStorage`](../tests/storage.go) constructs an isolated in-memory [`Storage`](../tests/storage.go) binding. Registering it as a constructor tests the complete Go-to-PHP bridge: automatic `context.Context` injection, methods, rich [`Record`](../tests/storage.go) return values, slices of records, and returned errors becoming PHP exceptions.

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

Each `new Storage` gets a new map. `get()` returns an error for a missing key, `all()` returns records sorted by key for deterministic assertions, and `len()` reports the number of records. `NewFailStorage` always returns `boom` for constructor-error tests. The fixture harness additionally injects the tenant `acme`, which `tenant()` exposes.

### Shared-memory binding

The production [`core.SharedMemory`](../stdlib/core/shared_memory.go) binding is also the test utility for state shared by otherwise independent runtimes, especially route requests. Create one value, place it in each runtime context, and register the standard library:

```go
shm := core.NewSharedMemory()

rt.SetContext(core.SharedMemoryContext(rt.Context(), shm))
stdlib.Register(rt)
```

PHP accesses this constructor as `SharedMemory`. The binding exposes `set` and `get` for string values, `incr` and `count` for named counters, and `delete`, `has`, and `clear` for cleanup and assertions. Reuse the same `shm` across runtimes to assert cross-request state; create a new one per test to avoid state leaking between tests.

## `.phpt` fixtures

Fixtures live in a per-area folder below [`tests/fixtures`](../tests/fixtures): `arithmetic`, `arrays`, `autoloading`, `bindings`, `exceptions`, `flatstack`, `functions`, `includes`, `oop`, `runtime`, `stdlib`, `strings` and `syntax`. The test harness discovers every file with a `.phpt` extension below that tree and runs it through the default `runner` runtime, and through the other runtimes the fixture has not opted out of. A new area is a new folder; nothing registers it.

A fixture's own folder is its include root. That is what lets all three runtimes agree: the `php` runner executes with its working directory set to the folder holding the fixture, and both Go runtimes are rooted at the same folder, so a relative path in the fixture names the same file whichever runtime reads it.

Because the folder is the unit of discovery, a bare directory path is not recursive. `phpscript test ./...` is what runs a tree; `phpscript test .` matches only the fixtures sitting directly in that directory, and reports an error rather than success when it matches none.

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

| Field         | Required | Purpose                                                                    |
|---------------|---------:|----------------------------------------------------------------------------|
| `name`        |      yes | Human-readable subtest name.                                               |
| `description` |      yes | Behavior and intent covered by the fixture.                                |
| `error`       |       no | Substring that must occur in the chain of an uncaught runtime error.       |
| `stdin`       |       no | String exposed to the script through `STDIN`.                              |
| `runner`      |       no | Runtimes the fixture opts out of; see [Runner metadata](#runner-metadata). |
| `root`        |       no | Include root, relative to the fixture's own directory.                     |

The expected-output section is always checked. Trailing newline differences are ignored, but all other output must match exactly. For an uncaught error, set `error` to a stable identifying substring and normally expect the host response body `Internal Server Error`:

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

A fixture that needs a tree phpscript does not embed names one with `root:`, resolved against the fixture's own directory. The runtime then reads that tree from disk instead of the embedded copy, and the `php` runner executes there too, so all three runners still agree. This is what lets a fixture load a composer `vendor/autoload.php`:

```phpt
name: renders a template
description: >
  ...
root: ..
---
<?php
require 'vendor/autoload.php';
```

Such a fixture gets its own include cache, because a cache is keyed by the path as the script wrote it and a fixture reaching a different tree must not be served a program cached for the embedded one.

Files used by `include`, autoloading, templates, or filesystem APIs sit inside the area folder that uses them, and fixture code names them relative to that folder: `autoloading/psr4/loader.php` is `psr4/loader.php` to a fixture in `autoloading`. Keeping the support files with their fixture is what keeps the include root a single directory, and a support file that two areas need is copied rather than shared, because an include path that climbs out of the fixture's folder is rejected.

### Checking a fixture against PHP

PHP 8.5 is installed as `/usr/bin/php`. Where a fixture uses only language features and PHP's own library, its expected-output section is what `php` prints for the same source, and that is verified rather than assumed. `phpscript test --matrix` runs that check for every fixture at once; to see the whole difference for one of them, run the two sides by hand. The two `---` lines make the sections addressable:

```bash
cd tests/fixtures/arrays
awk '/^---$/{n++; next} n==1' sort.phpt > /tmp/fixture.php
awk '/^---$/{n++; next} n==2' sort.phpt > /tmp/want.txt
php /tmp/fixture.php > /tmp/got.txt
diff /tmp/want.txt /tmp/got.txt
```

An empty diff means the fixture holds phpscript to PHP's behavior. A difference is the fixture's answer to a question worth resolving before it lands: either phpscript is wrong, or the fixture's expectation is.

Write the fixture this way around. Run the source through `php` first, paste that output into the expected section, and then make phpscript produce it. Writing the expected section from phpscript's output tests the runtime against itself.

Three kinds of fixture cannot be checked this way, and none of them is an excuse to skip the check on the ones that can:

| Fixture uses                                                     | Why `php` cannot run it                                                       |
|------------------------------------------------------------------|-------------------------------------------------------------------------------|
| A host binding (`Storage`, `Database`, `SharedMemory`, `tenant`) | The name does not exist in PHP; the expected output is the runtime's contract |
| Runtime introspection (`phpinfo`, `get_included_files`)          | The output names phpscript, or absolute paths that differ per machine         |
| Host request state (superglobals populated by the harness)       | The harness supplies the request, not the PHP CLI SAPI                        |

A fixture in one of these groups states in its `description` what defines the expected output, because there is no second implementation to appeal to, and opts the php runtime out with `runner`.

## Runner metadata

Every fixture runs through every runtime. A fixture that one runtime cannot execute opts that runtime out:

```yaml
runner:
  php: false
```

Both `flatstack` and `php` are accepted, and an omitted key means the runtime is used. The default runtime cannot be opted out of, because its output is what the expected-output section states.

Opt out only where the runtime has nothing to say about the fixture: `php: false` for the three groups above, `flatstack: false` where the bytecode engine cannot execute the program at all. Syntax the bytecode engine does not compile is not a reason on its own, because it falls back to the compatibility interpreter and produces the same output.

## Testing across runtimes

`phpscript test --matrix` runs every fixture through all three runtimes and prints one table per area, with the area name as the header of the fixture column:

```bash
phpscript test --matrix tests/fixtures/...
phpscript test --matrix -v tests/fixtures/...   # with the failures of each runtime
```

| arrays              | Flat stack | Runtime | PHP  |
|---------------------|------------|---------|------|
| array_indexing.phpt | PASS       | PASS    | PASS |
| sort.phpt           | PASS       | PASS    | PASS |

| bindings          | Flat stack | Runtime | PHP  |
|-------------------|------------|---------|------|
| storage_list.phpt | PASS       | PASS    | SKIP |

Each table is followed by that area's subtotal, and the run ends with the total.

A `SKIP` is a fixture that opted the runtime out, or a `php` binary that is not installed. Any other non-pass fails the run. This is the check that finds a divergence between the two Go runtimes, and between phpscript and PHP itself.

### Writing the report to a file

`-o` writes the same tables as Markdown while the terminal output continues as normal:

```bash
phpscript test --matrix -v -o ../../docs/test-fixtures.md ./...
```

That is what produces [test-fixtures.md](./test-fixtures.md), which `atkins test:phpscript:matrix` regenerates on every pipeline run. One run reports the suite and writes the report, so the fixtures are not executed twice to produce both. The report ends with a summary table whose total is the sum of the per-area rows.

`--profile`, `--count` and `--time` add their cost columns to the Markdown as well as the terminal. A matrix row has one cost column and three runners, so the numbers are the default runtime's: the matrix compares correctness across runtimes and cost on the runtime the other two are measured against, and `--json` keeps the per-runtime figures. The checked-in report is generated without them, because a timing that differs by a millisecond per run would be a diff in every commit.

## Running fixture tests

Run all default-runtime fixtures:

```bash
go test ./tests -run '^TestFixtures$'
```

Run every fixture through the flat bytecode runtime:

```bash
go test ./tests -run '^TestFlatstackFixtures$'
```

Subtests are nested under their area, so a `-run` pattern can select either:

```bash
go test ./tests -run 'TestFixtures/arrays'       # one area
go test ./tests -run 'TestFixtures/arrays/sort'  # one fixture
go test ./tests -run 'TestFlatstackFixtures/arrays/sort'
```

Before submitting a change, run the package tests affected by the change, then the complete suite with `go test ./...`, and then `phpscript test --matrix tests/fixtures/...`. A change to language or runtime behavior is not finished until it has a fixture, and a fixture covering PHP's own behavior is not finished until it passes the php column of the matrix.

## The pipeline

`atkins` runs the default pipeline: format, build, `go test`, the fixtures on all three runtimes, the introspection step that regenerates the generated documentation, and the docker image. It needs a Go toolchain, a `php` binary, and docker — for the mysql and postgres containers the database fixtures query, and for the image build. `db:up` starts those two services and the deferred `db:down` stops them, so a pipeline that fails partway still leaves nothing running.

`docker:build` is in the default pipeline rather than with the demos: the image is what `compose:up` and `compose:down` operate on and what a deployment ships, so a pipeline run leaves a current one behind whether or not anybody asked for the demos.

The demo *suites* are not in the default pipeline. `atkins test:demos` brings the compose stack up and runs a venom suite against each of [demos/dbadmin](../demos/dbadmin) and [demos/example](../demos/example). It runs whatever image is on the host rather than building one, so run `atkins` first when the demos have to exercise the tree rather than the last release.

```bash
atkins                     # the runtime, the docs and the image
atkins test:demos          # the demo applications, against the compose stack
atkins docker:build        # just the image
atkins test:phpscript:matrix   # just the fixture matrix
```
