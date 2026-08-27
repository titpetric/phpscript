# phpscript CLI

Install the command with Go:

```bash
go install github.com/titpetric/phpscript@latest
```

The binary is named `phpscript`. Running a PHP file is the default command, so
these are equivalent:

```bash
phpscript script.php
phpscript run script.php
```

You can use `titpetric/phpscript:latest` docker image (linux/amd64, ~44MB).

Use `phpscript -f config.yml ...` to load runtime and server settings from a
YAML file. Without `-f`, the binary uses its embedded defaults. See
[Configuration](./configuration.md) for every available setting.

## Commands

### `phpscript run <file.php>`

Parse and execute a PHP script in CLI mode.

```bash
phpscript run tests/fixtures/test-hello-world.php
phpscript tests/fixtures/test-hello-world.php
```

Use this command for normal script execution and shebang scripts:

```php
#!/usr/bin/env phpscript
<?php
echo "Hello world\n";
```

### `phpscript info [path...]`

Print the runtime environment, the way `phpinfo()` does in a terminal.

```bash
phpscript info
phpscript info -v
phpscript info ./src
```

With no path, the command prints the built-in runtime. `-v` / `--verbose`
adds bound classes (with constructors and methods) and internal functions.
A path argument uses the same file expansion as `list` and prints markdown
docs for classes and functions found in that tree.

### `phpscript lint <path>...`

Lint one or more PHP files or directories.

```bash
phpscript lint tests/fixtures/...
phpscript lint path/to/file.php
```

The lint pass reports two shapes:

| Finding                                               | Example                         |
|-------------------------------------------------------|---------------------------------|
| `assignment in conditional statement`                 | `if (($row = fn()) !== false)`  |
| `chained assignment binds one value to several names` | `$inlines = $blocks = array();` |

Both are warnings; only a parse error fails the run. The chained-assignment rule
exists because phpscript arrays are handles rather than values, so the two names
end up sharing one array where PHP would give each its own. See
[Value semantics](reference/types/value-semantics.md#arrays-are-handles-not-values).

Findings are printed one per row, with a row per file that had none, using a
colored table in a terminal and Markdown when output is redirected. Use
`--output FILE` (`-o`) to write the same table to a file as Markdown while the
terminal output continues as normal.

```bash
phpscript lint -o docs/lint.md tests/fixtures/...
```

Use `--flatstack` to also report whether the flat bytecode engine can run each
file, which adds a failing row per file it cannot compile. See
[Flat stack](flatstack.md).

### `phpscript test <path>...`

Discover and run `.phpt` fixtures. With no path, the command searches the
current directory. Results are printed as they complete, using a colored table
in a terminal and Markdown when output is redirected.

Fixtures are grouped into one table per folder, with the folder name as the
header of the fixture column, and each table is followed by that folder's
subtotal. A directory path is not recursive on its own: `./...` walks a tree,
and a path that matches no fixture is an error rather than a silent pass.

```bash
phpscript test tests/fixtures/...
phpscript test tests/fixtures/arrays
phpscript test tests/fixtures/arrays/array_indexing.phpt
```

Use `--count N` (`-c`) to run each fixture N times in one aggregate row, or
`--time D` (`-t`) to repeatedly run each fixture for at least duration D.
When both flags are provided, they select benchmark sampling: each fixture
prints N rows, and each row is an independent sample that runs for at least D.
The `Count` column is the number of fixture executions completed during that
sample. With `--time`, `P50`/`P95`/`P99` report per-operation latency in
microseconds so the wall-clock sample window is not mistaken for a single-run
duration. `GC Runs` reports completed Go garbage-collection cycles as
`N (M%)`, where M is their share of the fixture execution count for that row,
shown to two decimal places.

```bash
phpscript test -c 5 tests/fixtures/...
phpscript test -t 1s tests/fixtures/...
phpscript test --count 5 --time 1s tests/fixtures/...
```

Use `--parallel N` (`-p`) to run up to N fixtures in each area concurrently.
Output remains in discovery order. Fixtures that share external state can set
`serial: true` in their metadata to run as a barrier between parallel batches.
`--profile` cannot be combined with parallel execution because Go's allocation
counters are process-wide and cannot be attributed to one concurrent fixture.

Use `--profile` to add per-operation allocation and byte counts. `--json`
writes a machine-readable report to stdout (no table). `--cpuprofile` and
`--memprofile` write pprof files for the whole `test` invocation.

Use `--output FILE` (`-o`) to write the same tables to a file as Markdown while
the terminal output continues as normal. The file ends with a summary table of
per-folder totals. `--matrix`, `--profile`, `--count` and `--time` all
contribute their columns to it.

```bash
phpscript test --matrix -o docs/test-fixtures.md tests/fixtures/...
```

Use `--matrix` to run every fixture through all three runtimes (the flat
bytecode engine, the default interpreter, and the `php` binary), reporting one
row per fixture with a cell per runtime. Every other `test` flag is honored,
and the benchmarking flags add their columns after the runtime columns; those
numbers are the default runtime's, because a row has one cost column and three
runtimes. Per-runtime cost stays available in `--json`. A
fixture that opts out of a runtime, and a runtime that is not installed, are
reported as `SKIP`; anything else that is not a pass fails the run, and the
command exits non-zero.

```bash
phpscript test --matrix tests/fixtures/...
phpscript test --matrix -v tests/fixtures/...
```

| arrays              | Flat stack | Runtime | PHP  |
|---------------------|------------|---------|------|
| array_indexing.phpt | PASS       | PASS    | PASS |
| sort.phpt           | PASS       | PASS    | PASS |

| bindings          | Flat stack | Runtime | PHP  |
|-------------------|------------|---------|------|
| storage_list.phpt | PASS       | PASS    | SKIP |

Add `--verbose` (`-v`) to print the failure of each runtime in continuation
rows below its fixture. A continuation row leaves the fixture column empty, so
it reads as part of the row above it.

### `phpscript fmt <path>...`

Format one or more PHP files or directories in place. A directory path formats
PHP files directly in that directory; append `/...` to include its
subdirectories. With no path, the command uses the current directory (`.`).

```bash
phpscript fmt script.php
phpscript fmt ./src        # PHP files directly in ./src
phpscript fmt ./src/...    # PHP files in ./src and its subdirectories
phpscript fmt -l ./src/... # list what needs formatting, rewrite nothing
```

The formatter uses tabs for indentation, keeps class, function and
control-statement opening braces on the declaration line, and normalizes line
endings to LF. Class members are printed as constants, then properties, then
methods, keeping the blank lines written between them. An array literal with
more than two key/value pairs, or one that does not fit in 100 columns, is
printed one entry per line with a trailing comma. Comments, the quoting of
string literals, type hints and imports are kept as they were written.

A file the formatter cannot read in full is reported on standard error and
left alone, and the remaining files are still formatted:

```
$ phpscript fmt ./vendor/...
vendor/titpetric/minitpl/code/MiniTPL/Compiler.php
skipped vendor/titpetric/minitpl/test/TemplateTest.php: line 3: expected "{", got 3("extends")@3
```

Before a file is rewritten, its formatted output has to parse and formatting
it again has to produce the same text. A file that fails either check is
skipped rather than written.

### `phpscript list <path>...`

List routes, PHP files, and classes found under one or more paths as a Markdown
table. When output is attached to a terminal, the table is trimmed to fit its
width. A directory path lists PHP files directly in that directory; append
`/...` to include its subdirectories. With no path, the command uses the
current directory (`.`).

```bash
phpscript list ./src        # PHP files directly in ./src
phpscript list ./src/...    # PHP files in ./src and its subdirectories
phpscript list index.php    # A specific PHP file
```

The Route column names the entry point a file provides: a `METHOD /path`
annotation, `@startup` for a file the server runs before it listens, or
`@schedule ...` for a clock or interval job. Files that are only included by
others have neither.

### `phpscript ast <file.php>`

Tokenize a PHP file and print its PHP-style token stream.

```bash
phpscript ast tests/fixtures/syntax/code/TestCase.php
```

The output uses the same token names exposed by `token_get_all()` and
`token_name()`, such as `T_OPEN_TAG`, `T_STRING`, `T_VARIABLE`, and
`T_OBJECT_OPERATOR`, plus `CHAR` for single-character tokens. Each line includes
the source line number, token name, and raw token text.

This is a debugging/development helper. It is useful when checking how PHP
source is tokenized before changing parser or runtime behavior.

### `phpscript server [directory]`

Run the PHP application rooted at the given directory. The directory defaults
to the current working directory and must contain a `public/` web root.

```bash
phpscript server
phpscript server ./my-app
```

CSS, JavaScript, images, and other non-PHP files are served directly from
`public/`. PHP files in `public/` are executable by filename, and a request that
names a directory resolves to its `index.php`, or to its `index.html` when there
is no `index.php`: `/` is `public/index.php` or `public/index.html`, and `/docs/`
is `public/docs/index.php` or `public/docs/index.html`. A directory with neither
is a 404 unless the configuration sets `autoindex: true`, which answers it with a
generated listing instead. Files outside `public/` are never directly exposed.
See [Serving static files](use-cases/static-files.md).

A file in `public/` named after a status answers for it: `public/404.php` is
what a visitor following a dead link sees, and `public/503.php` answers a
`throw new Exception("...", 503)`. Writing the file is all there is to it, and a
site with no such file answers the way it did before. Programs are not sent one:
a `fetch()`, an API client and curl get the plain status, as does any endpoint
that wrote a body or declared a `Content-Type`. See [Error
pages](use-cases/error-handling.md#error-pages).

PHP files outside `public/` are scanned recursively for
`// @route METHOD /path/{param}` annotations. Those routes execute with the
application directory as their source filesystem, so they can include shared
bootstrap code and templates outside the web root. Route annotations in
`public/` are ignored. Route loading is controlled by the active configuration:

```yaml
routes:
  enabled: true
```

Before the server starts listening, PHP files anywhere in the application tree
that contain an `@startup` comment execute once in path order. Use these files
for required setup such as database migrations:

```php
<?php
// @startup

$migrate = new Database\Migrate("app");
$migrate->load("./schema/*.up.sql");
$migrate->run();
```

`@schedule` annotations start after listen and keep running until shutdown:

```php
<?php
// @schedule daily -- prune
// @schedule every 5 minutes -- sync

switch ($argv[1]) {
case "prune":
	break;
case "sync":
	break;
default:
	echo "Unknown/missing command";
	exit(1);
}
```

Specs: `every N seconds|minutes|hours`, `hourly`, `daily`, `weekly`,
`monthly`, `every weekday`, `every sunday` (any weekday), `N times per hour|day`.
Arguments after `--` become `$argv[1:]`. Calendar specs fire at local midnight.
A tick is skipped if the previous run is still going. Output is stored on the
oida span as `output`.

Startup files run with the CLI SAPI and the application filesystem. If one
fails, the server is not started. See
[Run migrations](./use-cases/database.md#run-migrations) for the schema layout
these files apply.

```text
my-app/
├── public/
│   ├── index.php
│   └── assets/
│       ├── app.js
│       └── style.css
├── routes/
│   └── users.php
└── templates/
    └── user.tpl
```

See [PHP routing](./use-cases/routing.md) for route annotation details.

### `phpscript version`

Print build and module information.

```bash
phpscript version
```

## Docker image

Build a local binary and image from source:

```bash
CGO_ENABLED=0 go build -o bin/ .
docker build -t titpetric/phpscript:latest -f docker/Dockerfile .
```

See [../compose.yml](../compose.yml) for the development/test services used by
the repository.

## Embedding from Go

The CLI is a thin wrapper around the Go runtime. For applications that need host
bindings, construct a `runner.Runtime` directly and register functions,
constructors, or request context values from Go code.

```go
rt := runner.New(os.Stdout, runner.Options{RootFS: os.DirFS(".")})
rt.RegisterConstructor("Storage", NewStorage)
```

This enables PHP code such as `new Storage` to use Go-backed values. Constructor
and method parameters can receive `context.Context` automatically when the Go
function signature asks for it, and returned errors are surfaced to PHP as
runtime exceptions.
