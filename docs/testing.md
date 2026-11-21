# Testing phpscript

Run the full test suite from the repository root:

```bash
go test ./...
```

A change to language or runtime behavior lands with a `.phpt` fixture. A Go test in the package that owns the behavior covers assertions needing direct access to Go APIs, parser models, runtime state, concurrency, or error types. Add the fixture as well. A Go test proves the Go code matches its author's intent. A fixture proves a script sees it.

A Go test file takes the name of the file it covers: `dispatch_test.go` beside `dispatch.go`. That is the pairing the `pairing` linter reads. Two test files covering one source stay one test file. A suite reaching the package only through the public API names no file. A behaviour table over the engine, a benchmark arrangement, a regression per issue goes under [`tests/`](../tests) in a separate folder: `tests/runner`, `tests/flatstack`, `tests/model`, `tests/github`. No file there takes a source file's name, and the linter excuses the directory.

`.splint.yml` caps a file at 400 lines and names four paths that cap higher. A test file cannot split freely. `pairing/orphan` wants `foo_test.go` beside `foo.go`. One source file's whole contract lands in a single test file. Its inputs and failure messages run longer than the code they cover. `parser/*.go` covers one subject, the grammar. A split by line count would put the statement that opens a block in one file and the statement that closes it in another. `flatstack/engine/*.go` holds the instruction set and the loop that runs it. That loop over the switch of every opcode keeps it quick. `model/ast.go` holds the node set the parser writes. Both engines read it. `grouping/filename` exempts a file that compiles by itself, which this one does, unlike a split file. The choice stands between fifty six-line files and this one. This fifth entry argues the point separately.

A fixture does two jobs and must do both:

1. It states the behavior. A change that alters it fails a named test, ahead of any benchmark or demo.
2. It records what PHP itself produces. The fixture measures the runtime against the language, not against the runtime's own output.

The second job finds defects. Expected output written from what phpscript prints locks in whatever it prints, including the wrong parts. Expected output taken from `php` makes the fixture a compatibility check.

## Go test utilities

The [`tests` package](../tests) holds reusable bindings and setup for tests that need to exercise the runtime from another Go package. Import it with an alias to make clear that these are repository test helpers, not production APIs:

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

A `PLATFORM_DB_<NAME>=<driver>://<dsn>` variable names a connection. The name a script opens is that name lowercased. `TestMain` builds a provider from the process environment, with these read on top. A `go test` run has them even when the shell exported none:

| Environment variable        | `new Database(...)` | Configured test service          |
|-----------------------------|---------------------|----------------------------------|
| `PLATFORM_DB_SQLITE_TEST`   | `sqlite_test`       | Shared in-memory SQLite database |
| `PLATFORM_DB_POSTGRES_TEST` | `postgres_test`     | PostgreSQL on `localhost:15432`  |
| `PLATFORM_DB_MYSQL_TEST`    | `mysql_test`        | MySQL on `localhost:13306`       |
| `PLATFORM_DB_SCAFFOLD`      | `scaffold`          | Shared in-memory SQLite database |

It builds a provider without writing to the process environment. The PostgreSQL service is [`docker/service/postgres.yml`](../docker/service/postgres.yml) and the MySQL service is [`docker/service/mysql.yml`](../docker/service/mysql.yml), both included by [`compose.yml`](../compose.yml). `TestMain` calls `os.Exit` and call it as the external package's complete `TestMain`, never from an individual test.

`phpscript test` is a different process and reads none of that. It builds connections from the environment plus the `env` list of the configuration covering it. Those pipeline jobs run the binary with [`.env.testing`](../.env.testing):

```yaml
test:phpscript:matrix:
  env:
    include: .env.testing
```

A fixture area can name connections too. [`tests/fixtures/scaffold`](../tests/fixtures/scaffold) does. See [Suite configuration](#suite-configuration).

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

The production [`core.SharedMemory`](../stdlib/ps/core/shared_memory.go) binding is also the test utility for state shared by otherwise independent runtimes, especially route requests. Create one value, place it in each runtime context, and register the standard library:

```go
shm := core.NewSharedMemory()

rt.SetContext(core.SharedMemoryContext(rt.Context(), shm))
stdlib.Register(rt)
```

PHP accesses this constructor as `SharedMemory`. The binding exposes `set` and `get` for string values, `incr` and `count` for named counters, and `delete`, `has`, `clear` for cleanup and assertions. Reuse the same `shm` across runtimes to assert cross-request state. Create a new value for each test to avoid state leaking between tests.

### `t.Parallel()`

A top-level test declares `t.Parallel()` as its first statement unless something below disqualifies the test. The suite runs in parallel. A new test follows suit so one serial straggler does not stall the package.

A test stays serial when it:

- calls `t.Setenv` (Go panics on it in a parallel test), `t.Chdir`, `os.Chdir`, or swaps `os.Stdout`, `os.Stderr` or `os.Stdin`.
- writes process-global state, directly or through a helper it calls: `stdlib.SetProfile` (reached through `flags.Options.Validate`), `lint.SetInclude` (reached through the lint command's `run`), `slog.SetDefault`, the binding registry, a package-level variable.
- asserts on a process-wide counter: `testing.AllocsPerRun`, `runtime.ReadMemStats`, a live-heap delta recorded by `Precompiler.Run`, a goroutine count.
- runs fixtures through the `tests` package: `RunFixture` outside a worker loop lands on worker zero. All such callers share one set of caches plus one runtime. The suite's scaffold connections point at one shared in-memory sqlite.
- bounds elapsed time from above, since four saturated cores stretch every sleep, every scheduling gap.
- inspects a value after handing it to a `sync.Pool`, where a concurrent draw can mutate it.

The disqualifying call often sits two frames down, and the check follows helpers, not just the test body. Prove a parallelised package with a `go test -count 2` run plus a separate `-race` run. The first catches shared fixtures. The second catches globals the code read missed. A package that resists stays serial, with a reason recorded. The `cmd/phpscript/server` vhost tests are serial: a recycled tracer span context reaching sqlite's interrupt watcher segfaults under reordering.

A parent that calls `t.Parallel()` returns before its parallel children run and cleanup written after a `t.Run` loop of parallel subtests runs too early. Register it with `t.Cleanup` instead.

## `.phpt` fixtures

Fixtures go in a per-area folder below [`tests/fixtures`](../tests/fixtures): `arithmetic`, `arrays`, `autoloading`, `bindings`, `comparison`, `errors`, `exceptions`, `flatstack`, `functions`, `gd`, `includes`, `mail`, `namespaces`, `oop`, `output`, `paths`, `pexec`, `regex`, `routing`, `runtime`, `scaffold`, `stdlib`, `strings`, `syntax` and `types`. The test harness discovers every `.phpt` file below that tree. It runs each through the default `runner` runtime plus the other runtimes the fixture did not opt out of. A new area is a new folder that nothing registers. [`github`](../tests/fixtures/github) is the exception: it holds issue reproductions written as `.yml` files, and nothing there runs.

A fixture's own folder is the include root. Every runtime resolves includes against the fixture's own folder. The `php` runner executes with its working directory set to that folder. Both Go runtimes take the same folder as root. A relative path names the same file whichever runtime reads it.

Because the folder is the unit of discovery, a bare directory path is not recursive. A tree runs with `phpscript test ./...`. `phpscript test .` matches only the fixtures sitting directly in that directory, and reports an error when it matches none. `phpscript test` alone runs the whole tree below the working directory. A pipeline invoked from an application root means this.

An application root can supply one bootstrap that every fixture loads. `--include vendor/autoload.php` includes the named file, resolved against the invocation root, before each fixture body. The flag applies when the file exists. The same pipeline line works in a tree holding no bootstrap. composer's autoloader resolves the classes. The file's own includes bring the helpers. A fixture names neither. Keep its folder as the include root: its relative includes answer first. The invocation root answers for anything the folder does not hold.

A tree that would repeat that flag on every invocation writes it down instead. See [Suite configuration](#suite-configuration).

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

| Field         | Required | Purpose                                                                                                                |
|---------------|---------:|------------------------------------------------------------------------------------------------------------------------|
| `name`        |      yes | Human-readable subtest name.                                                                                           |
| `description` |      yes | Behavior and intent covered by the fixture.                                                                            |
| `error`       |       no | Substring that must occur in the chain of an uncaught runtime error.                                                   |
| `stdin`       |       no | String exposed to the script through `STDIN`.                                                                          |
| `runner`      |       no | Runtimes the fixture opts out of; see [Runner metadata](#runner-metadata).                                             |
| `root`        |       no | Include root, relative to the fixture's own directory.                                                                 |
| `serial`      |       no | Do not overlap the fixture with peers under `--parallel`.                                                              |
| `options`     |       no | Runner options for both Go runtimes, spelled as the `runner` configuration block.                                      |
| `runtime`     |       no | The runtime namespace, `phpscript` or `go`; it wins over `options.runtime`.                                            |
| `request`     |       no | Request state the harness exposes as superglobals: `args`, `get`, `post`, `cookie`, `env`, `headers`, `stdin`, `body`. |
| `response`    |       no | Response headers the run must have set.                                                                                |

The expected-output section is always checked. The harness ignores trailing newline differences, but all other output must match exactly. For an uncaught error, set `error` to a stable identifying substring and normally expect the host response body `Internal Server Error`:

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

A fixture that needs a tree phpscript does not embed names one with `root:`, resolved against the fixture's own directory. The runtime then reads that tree from disk in place of the embedded copy. The `php` runner executes there too. All three runners still agree. This lets a fixture load a composer `vendor/autoload.php`:

```phpt
name: renders a template
description: >
  ...
root: ..
---
<?php
require 'vendor/autoload.php';
```

The include root keys the caches. A cache keys on the path as the script wrote it. Two roots can both hold a `code/functions.php`. A fixture reaching a different tree must not get a program cached for the embedded copy. The key is absolute: the relative spelling is ambiguous once the working directory moves.

`--cache` controls the reach of a cached program. The default, `worker`, keeps one set of caches plus one runtime per worker loop. That worker reuses them across the fixtures it runs serially. A run's memory scales with `--parallel`, not with the fixture count. `--cache=off` builds a set for each fixture run. It drops the set and its runtime when the run ends. A fixture's parsed or declared state does not reach the next run. That flag suits a fixture that passes alone and fails in the suite.

Files used by `include`, autoloading, templates, or filesystem APIs sit inside the area folder that uses them. Fixture code names them relative to that folder: `autoloading/psr4/loader.php` is `psr4/loader.php` to a fixture in `autoloading`. Keeping support files beside their fixture keeps the include root a single directory. Copy a support file that two areas need into each. The harness rejects an include path that climbs out of the fixture's folder.

### Checking a fixture against PHP

PHP 8.5 is at `/usr/bin/php`. Where a fixture uses only language features and PHP's own library, its expected-output section holds what `php` prints for the same source, and the matrix verifies it. `phpscript test --matrix` runs that check for every fixture at once. To see the whole difference for one of them, run the two sides by hand. The two `---` lines make the sections addressable:

```bash
cd tests/fixtures/arrays
awk '/^---$/{n++; next} n==1' sort.phpt > /tmp/fixture.php
awk '/^---$/{n++; next} n==2' sort.phpt > /tmp/want.txt
php /tmp/fixture.php > /tmp/got.txt
diff /tmp/want.txt /tmp/got.txt
```

An empty diff means the fixture holds phpscript to PHP's behavior. A difference is the fixture's answer to a question worth resolving before it lands: either phpscript is wrong, or the fixture's expectation is.

A fixture that ends its PHP section with `?>` needs no extraction at all. Everything after the tag is trailing text the interpreter echoes and PHP runs the file as-is:

```bash
php tests/fixtures/arrays/sort.phpt
```

Output holds the metadata, `---`, the actual result, `---`, the expected result. The two sections are adjacent. A mismatch is visible by eye. **Close the tag by default**: use `awk` when a machine diffs the sections, `php <fixture>` when a person does.

`php` still cannot run three kinds of fixture: their names, paths or request state exist only inside the harness. They close the tag anyway, for one file shape.

Write the fixture this way around. Run the source through `php` first, paste that output into the expected section, and then make phpscript produce it. Writing the expected section from phpscript's output tests the runtime against itself.

Three kinds of fixture fall outside the php check. It still applies wherever `php` can run the fixture:

| Fixture uses                                                             | Why `php` cannot run it                                                       |
|--------------------------------------------------------------------------|-------------------------------------------------------------------------------|
| A host binding (`Storage`, `Database`, `Mail`, `SharedMemory`, `tenant`) | The name does not exist in PHP; the expected output is the runtime's contract |
| Runtime introspection (`phpinfo`, `get_included_files`)                  | The output names phpscript, or absolute paths that differ per machine         |
| Host request state (superglobals populated by the harness)               | The harness supplies the request, not the PHP CLI SAPI                        |

A fixture in one of these groups states in its `description` what defines the expected output, because no other implementation exists to appeal to, and opts the php runtime out with `runner`.

## `_test.php` fixtures

The second form uses two files. `<name>_test.php` holds the body. `<name>_test.txt` holds the output all three runtimes must produce. `phpscript test` collects the pair from the same paths as a `.phpt`, with the same flags, and reports it in the same tables.

`tests/fixtures/paths/api/magic_scope_test.php`:

```php
<?php

echo __LINE__, "\n";
echo basename(__FILE__), "\n";
echo basename(__DIR__), "\n";

class Widget {
	public function instance() {
		return __FUNCTION__ . "|" . __CLASS__ . "|" . __METHOD__;
	}
}
echo (new Widget)->instance(), "\n";
```

`tests/fixtures/paths/api/magic_scope_test.txt`:

```
3
magic_scope_test.php
api
instance|Widget|Widget::instance
```

The body is an ordinary php file. Two things follow from that.

Every runtime reads it from where it lies, the php column included. `__FILE__` and `__DIR__` compile to a path all three agree on. A `.phpt` body is a section of a document. The php column runs a throwaway copy with a name no other file sees. A fixture reading either constant cannot assert the file name.

It also runs by hand. `php tests/fixtures/paths/api/magic_scope_test.php` executes the body. Redirect the output into the `.txt`:

```sh
cd tests/fixtures/paths/api && php magic_scope_test.php > magic_scope_test.txt
```

The same rule governs a `.phpt` expected section: the output is php's, pasted, not phpscript's.

The form holds no frontmatter. A fixture that needs `runner`, `request`, `options`, `root` or `serial` is a `.phpt`. A `_test.php` without a `_test.txt` stays a php file someone put in the tree. `phpscript test` does not collect it.

## Suite configuration

A folder that needs a bootstrap, a connection or a schema writes it down once, off every command line reaching the folder. A `phpscript.yml` in the fixture tree marks a **suite root**: the directory whose fixtures run below that file. A fixture resolves the nearest such file at or above the fixture directory. The run resolves the nearest one at or above the working directory. A tree holding none behaves as before. This is a folder opting in.

[Test suites](configuration.md#test-suites) documents the block key by key. A suite root names the prelude its fixtures load, the connections they resolve, plus two hook files:

```yaml
env:
  - "PLATFORM_DB_SCAFFOLD=sqlite://file:phpscript-scaffold?mode=memory&cache=shared"

test:
  hooks:
    setup: setup.php
```

Both hooks run once per session, not once per fixture. `setup` lays down the schema and the rows its fixtures assert on. The run fails before any fixture executes when `setup` fails. `teardown` runs after the fixtures, whether they passed or failed. Their output reaches the terminal only under `-v`.

[`tests/fixtures/scaffold`](../tests/fixtures/scaffold) is the worked example. The area holds its configuration, its schema, a setup hook that migrates and seeds, and the fixtures that read the seeded rows:

```text
phpscript.yml               the connection and the hook
setup.php                   the migration, then the rows
schema/0001_catalogue.up.sql
catalogue_rows.phpt         the seeded rows
catalogue_lookup.phpt       a parameterised get() against one of them
catalogue_count.phpt        aggregates over the seeded table
catalogue_insert.phpt       serial: true, writes and removes what it wrote
```

It runs with nothing on the command line:

```bash
phpscript test tests/fixtures/scaffold
```

Writing a database area this way keeps its fixtures out of everyone else's flags and lets them stay parallel. Three of the four here read the seed and need no `serial:`, since none builds the state the others depend on. Only the writer is serial.

Seed the rows from the hook. A `.up.sql` is the wrong place. `Database\Migrate` records a migration, never applying it twice. A seed inside one lands in a database that has never seen it. That hook deletes and reinserts, leaving the same rows whatever state it found. Where a fixture asserts on generated ids, reset the sequence too: sqlite's `AUTOINCREMENT` keeps its high-water mark in `sqlite_sequence`. A delete does not return it.

`@startup` is not this mechanism. Annotations are server surface: `phpscript server` scans `@route`, `@startup` and `@schedule` out of a source tree, per virtual host or per application root depending on configuration. A fixture run is neither scope.

## Runner metadata

Every fixture runs through every runtime. A fixture that one runtime cannot execute opts that runtime out:

```yaml
runner:
  php: false
```

`runner` accepts `flatstack` and `php`. An omitted key runs that runtime. A fixture cannot opt the default runtime out, because the expected-output section states its output. `runtime: go` (or `options.runtime: go`) opts the php runner out, because php cannot run source written for the go runtime. `runner: {php: true}` is the only way back in.

Opt out only when the runtime plays no part in the fixture: `php: false` for those three fixture groups above, `flatstack: false` where a program the bytecode engine compiles must still not run there. Syntax the bytecode engine does not compile needs no opt-out: the harness compiles the program through the flat compiler before running, into the run's own cache so the run reuses the result, and reports the fixture as SKIP for that column. It does not delegate to the compatibility interpreter, which embedding does, since a fallback run would record the interpreter's result and cost in the flatstack column.

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

That area's subtotal follows each table, and the run ends with the total.

A `SKIP` is a fixture that opted the runtime out, or a `php` binary that is not installed. Any other non-pass fails the run. This is the check that finds a divergence between the two Go runtimes, and between phpscript and PHP itself.

### Writing the report to a file

`-o` writes the same tables as Markdown while the terminal output continues as normal:

```bash
phpscript test --matrix -v -o docs/test-fixtures.md ./tests/...
```

`atkins gen:fixtures` runs exactly that, spelled as above, to produce the [test fixture results](https://github.com/titpetric/phpscript/releases/latest/download/test-fixtures.md). The report ends with a summary table whose total is the sum of the per-area rows.

That is a separate job from the gate. `atkins test:phpscript:matrix` runs the same matrix on every pipeline invocation and writes nothing, since the report changes on every run. `.gitignore` covers the report. It travels as a release asset, which `atkins publish` attaches. Running the suite a second time only for the report is the cheaper half of that trade.

`--profile`, `--count` and `--time` add cost columns to the Markdown and the terminal. A matrix row holds one cost column across three runners, and the numbers come from the default runtime. The matrix compares correctness across runtimes and cost on the default runtime, the baseline for the other two. `--json` keeps the per-runtime figures. `atkins gen:fixtures` writes the report without cost columns: a timing that differs by a millisecond per run would be a diff in each commit.

## Running fixture tests

Run all default-runtime fixtures:

```bash
go test ./tests -run '^TestFixtures$'
```

Run every fixture through the flat bytecode runtime:

```bash
go test ./tests -run '^TestFlatstackFixtures$'
```

Both tests nest their subtests by area. A `-run` pattern selects either:

```bash
go test ./tests -run 'TestFixtures/arrays'       # one area
go test ./tests -run 'TestFixtures/arrays/sort'  # one fixture
go test ./tests -run 'TestFlatstackFixtures/arrays/sort'
```

Before submitting a change, run the package tests affected by the change, then the complete suite with `go test ./...`, then the matrix:

```bash
go install .                                    # the matrix runs the installed binary
phpscript test --matrix tests/fixtures/...
```

`go install .` first. `phpscript test` runs the installed binary from `PATH`. An edited runtime that nobody installs runs in its last installed state, and the fixtures pass or fail on stale code. `go test ./tests` has no such gap: it compiles the tree in process. Checkouts share `GOBIN`. A second clone installing over the same path shows the same symptom.

A change to language or runtime behavior is not finished until it has a fixture. A fixture covering PHP's own behavior is not finished until it passes the php column of the matrix.

## Issue reproductions

[`tests/fixtures/github`](../tests/fixtures/github) holds one file per GitHub issue, in the `.phpt` format with a `.yml` extension. No file there runs: the fixture runner discovers `.phpt` only. The `.yml` extension keeps a file written against *expected* behaviour out of the passing suite. The files stay after their issue closes, as the record of the report.

An issue whose behaviour a fixture can reach closes by renaming its file to `.phpt` and moving it to the area it belongs to. It joins the matrix from there.

An issue whose behaviour a fixture cannot reach closes with a Go test in [`tests/github`](../tests/github), one file per issue, `issue_NNN_test.go` holding `Test_IssueNNN`. Route registration, server flags and host bindings all sit outside script observation, and the Go tests there cover that surface. The `.yml` file stays. Its description names the test.

```text
tests/fixtures/github/62.yml     the transcript that was reported
tests/github/issue_062_test.go   Test_Issue062, the check that fails on a regression
```

## The pipeline

`atkins` runs the fast gate: format, lint, `go test`, `go install`, the introspection step that regenerates documentation, `mdox:fmt` across the markdown, and `bake` for both architectures. It needs a Go toolchain and docker. It runs no php and pushes nothing.

`atkins ci:daily` runs the suites a push cannot: bake, the fixtures on all three runtimes, the lifecycle run and the demo suites. It needs a `php` binary and docker. Docker runs the mysql and postgres containers the database fixtures query, and builds the image. `db:up` starts those two services. The deferred `db:down` stops them. Any run that fails partway still leaves nothing running.

`bake` builds `docker/Dockerfile.build` for amd64 and arm64, and `atkins deploy` pushes `titpetric/phpscript:latest-amd64` and `latest-arm64`, tags that follow `main` and are unstable. The gate ends with `bake`, `ci:daily` deploys before the demo suites, and `compose:up` and `compose:down` operate on that image.

The demo *suites* are not in the default pipeline. `ci:daily` runs `test:demos`, which brings the compose stack up and runs a venom suite against each of [demos/dbadmin](../demos/dbadmin) and [demos/example](../demos/example). It runs whatever image the host has. Run `atkins bake` first when the demos must exercise the tree. Otherwise they test the last release.

```bash
atkins                     # the fast gate: source, unit suite, generated docs
atkins ci:daily            # the image and the integration suites, out of band
atkins test:demos          # the demo applications, against the compose stack
atkins bake                # build both architecture images
atkins deploy              # build and push the image
atkins test:phpscript:matrix   # just the fixture matrix
```
