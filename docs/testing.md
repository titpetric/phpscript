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

Fixtures live in a per-area folder below [`tests/fixtures`](../tests/fixtures): `arithmetic`, `arrays`, `autoloading`, `bindings`, `errors`, `exceptions`, `flatstack`, `functions`, `gd`, `includes`, `namespaces`, `oop`, `output`, `paths`, `pexec`, `regex`, `runtime`, `stdlib`, `strings` and `syntax`. The test harness discovers every file with a `.phpt` extension below that tree and runs it through the default `runner` runtime, and through the other runtimes the fixture has not opted out of. A new area is a new folder; nothing registers it.

A fixture's own folder is its include root. That is what lets all three runtimes agree: the `php` runner executes with its working directory set to the folder holding the fixture, and both Go runtimes are rooted at the same folder, so a relative path in the fixture names the same file whichever runtime reads it.

Because the folder is the unit of discovery, a bare directory path is not recursive. `phpscript test ./...` is what runs a tree; `phpscript test .` matches only the fixtures sitting directly in that directory, and reports an error rather than success when it matches none. `phpscript test` with no path at all runs the whole tree below the working directory, the way a pipeline invoked from an application root means it.

An application root can supply its bootstrap to every fixture. `--include vendor/autoload.php` includes the named file, resolved against the invocation root, before each fixture body — when the file exists, so the same pipeline line works in a tree that has no bootstrap. It is the whole of it: composer's autoloader resolves the classes and the file's own includes bring the helpers, so a fixture names neither. The fixture's own folder stays its include root: its relative includes answer first, and the invocation root answers for what the folder does not hold.

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

| Field         | Required | Purpose                                                                      |
|---------------|---------:|------------------------------------------------------------------------------|
| `name`        |      yes | Human-readable subtest name.                                                 |
| `description` |      yes | Behavior and intent covered by the fixture.                                  |
| `error`       |       no | Substring that must occur in the chain of an uncaught runtime error.         |
| `stdin`       |       no | String exposed to the script through `STDIN`.                                |
| `runner`      |       no | Runtimes the fixture opts out of; see [Runner metadata](#runner-metadata).   |
| `root`        |       no | Include root, relative to the fixture's own directory.                       |
| `serial`      |       no | Do not overlap the fixture with peers when `--parallel` is enabled.          |
| `plugins`     |       no | Go plugins the fixture loads; see [Loading Go plugins](#loading-go-plugins). |

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

Caches are keyed by include root, because a cache is keyed by the path as the script wrote it and two roots can both hold a `code/functions.php` — a fixture reaching a different tree must not be served a program cached for the embedded one. The key is absolute, since the relative spelling is ambiguous once the working directory moves.

How far a cached program travels is `--cache`. The default, `worker`, gives each worker loop one set of caches and one runtime, reused by the fixtures that worker runs serially: what a run holds scales with `--parallel` rather than with the number of fixtures. `--cache=off` gives every fixture run its own and drops them, and its runtime, when the run ends — nothing one fixture parsed or declared is visible to the next. That is the flag to reach for when a fixture passes alone and fails in the suite.

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

A fixture that ends its PHP section with `?>` needs no extraction at all. Everything after the tag is trailing text the interpreter echoes, so PHP runs the file as-is:

```bash
php tests/fixtures/arrays/sort.phpt
```

Output: the metadata, `---`, what the code did, `---`, what it should have done. The two sections are adjacent, so a mismatch is visible by eye. **Close the tag by default** — use `awk` when a machine diffs the sections, `php <fixture>` when a person does.

The four kinds of fixture below still cannot be read this way: their names, paths or request state only exist inside the harness. They close the tag anyway, for one file shape.

Write the fixture this way around. Run the source through `php` first, paste that output into the expected section, and then make phpscript produce it. Writing the expected section from phpscript's output tests the runtime against itself.

Four kinds of fixture cannot be checked this way, and none of them is an excuse to skip the check on the ones that can:

| Fixture uses                                                     | Why `php` cannot run it                                                       |
|------------------------------------------------------------------|-------------------------------------------------------------------------------|
| A host binding (`Storage`, `Database`, `SharedMemory`, `tenant`) | The name does not exist in PHP; the expected output is the runtime's contract |
| Runtime introspection (`phpinfo`, `get_included_files`)          | The output names phpscript, or absolute paths that differ per machine         |
| Host request state (superglobals populated by the harness)       | The harness supplies the request, not the PHP CLI SAPI                        |
| A Go plugin (`plugins:`)                                         | The classes come from a `.so` the `php` binary knows nothing about            |

A fixture in one of these groups states in its `description` what defines the expected output, because there is no second implementation to appeal to, and opts the php runtime out with `runner`.

## Runner metadata

Every fixture runs through every runtime. A fixture that one runtime cannot execute opts that runtime out:

```yaml
runner:
  php: false
```

Both `flatstack` and `php` are accepted, and an omitted key means the runtime is used. The default runtime cannot be opted out of, because its output is what the expected-output section states.

Opt out only where the runtime has nothing to say about the fixture: `php: false` for the four groups above, `flatstack: false` where the bytecode engine cannot execute the program at all. Syntax the bytecode engine does not compile is not a reason on its own, because it falls back to the compatibility interpreter and produces the same output.

## Loading Go plugins

A fixture can load compiled Go extensions, which register their symbols after the standard library has and therefore replace anything of the same name:

```yaml
plugins: ../../testdata/plugins/http/plugin.so
runner:
  php: false
```

The key takes a whitespace-separated string or a list, and a name without a `.so` suffix names a directory holding `plugin.so`. A relative name is looked for beside the fixture, then under the module root, then under the working directory, so one spelling works from `go test`, from `phpscript test` and from the repository root.

The `php` opt-out is required rather than conventional: `ParseFixture` refuses a fixture that loads a plugin and still runs against `php`, because the failure would otherwise appear as an unexplained matrix cell.

Plugin sources live under `tests/testdata/plugins`, outside the fixture tree, because the fixture tree is embedded wholesale (`//go:embed all:fixtures`) and each `.so` built beside its sources is several megabytes read from disk by `plugin.Open`; embedding one would cost that much in every binary importing the `tests` package.

`atkins plugins:build` builds them, and the harness rebuilds one whose `.so` is missing or older than a `.go` file beside it, so a plugin is always built by the toolchain that opens it. A host built without cgo cannot load a plugin at all, and those fixtures are skipped rather than failed, the same way a missing `php` binary is.

See [Go plugins](./reference/extensions/plugins.md) for the plugin side.

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

Before submitting a change, run the package tests affected by the change, then the complete suite with `go test ./...`, and then the matrix:

```bash
go install .                                    # the matrix runs the installed binary
phpscript test --matrix tests/fixtures/...
```

`go install .` is not optional. `phpscript test` runs the binary on `PATH`, not the tree, so an edited runtime that has not been installed is tested in its previous state and the fixtures pass or fail on code that is no longer there. `go test ./tests` has no such gap: it compiles the tree in process. `GOBIN` is also shared between checkouts, so a second clone of this repository installing over the same path produces the same symptom.

A change to language or runtime behavior is not finished until it has a fixture, and a fixture covering PHP's own behavior is not finished until it passes the php column of the matrix.

## Issue reproductions

[`tests/fixtures/github`](../tests/fixtures/github) holds one file per GitHub issue, in the `.phpt` format under a `.yml` extension. Nothing there runs: the fixture runner discovers `.phpt` only, and the extension is what keeps a file written against *expected* behaviour out of the passing suite. The files stay after their issue closes, as the log of what was reported.

An issue whose behaviour a fixture can reach closes by renaming its file to `.phpt` and moving it to the area it belongs to. It joins the matrix from there.

An issue whose behaviour a fixture cannot reach closes with a Go test in [`tests/github`](../tests/github), one file per issue, `issue_NNN_test.go` holding `Test_IssueNNN`. Route registration, server flags and host bindings are all outside what a script can observe, so they are covered there. The `.yml` stays and its description names the test.

```text
tests/fixtures/github/62.yml     the transcript that was reported
tests/github/issue_062_test.go   Test_Issue062, the check that fails on a regression
```

## The pipeline

`atkins` runs the default pipeline: format, build, `go test`, the fixtures on all three runtimes, the introspection step that regenerates the generated documentation, and the docker image. It needs a Go toolchain, a `php` binary, and docker. Docker runs the mysql and postgres containers the database fixtures query, and builds the image. `db:up` starts those two services and the deferred `db:down` stops them, so a pipeline that fails partway still leaves nothing running.

`docker:build` is in the default pipeline rather than with the demos: the image is what `compose:up` and `compose:down` operate on and what a deployment ships, so a pipeline run leaves a current one behind whether or not anybody asked for the demos.

The demo *suites* are not in the default pipeline. `atkins test:demos` brings the compose stack up and runs a venom suite against each of [demos/dbadmin](../demos/dbadmin) and [demos/example](../demos/example). It runs whatever image is on the host rather than building one, so run `atkins` first when the demos have to exercise the tree rather than the last release.

```bash
atkins                     # the runtime, the docs and the image
atkins test:demos          # the demo applications, against the compose stack
atkins docker:build        # just the image
atkins test:phpscript:matrix   # just the fixture matrix
```
