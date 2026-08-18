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

### `phpscript lint <path>...`

Lint one or more PHP files or directories.

```bash
phpscript lint tests/fixtures
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

### `phpscript test <path>...`

Discover and run `.phpt` fixtures. With no path, the command searches the
current directory. Results are printed as they complete, using a colored table
in a terminal and Markdown when output is redirected.

```bash
phpscript test tests/fixtures
phpscript test tests/fixtures/array_indexing.phpt
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
phpscript test -c 5 tests/fixtures
phpscript test -t 1s tests/fixtures
phpscript test --count 5 --time 1s tests/fixtures
```

Use `--profile` to add per-operation allocation and byte counts. `--json`
writes a machine-readable report to stdout (no table). `--cpuprofile` and
`--memprofile` write pprof files for the whole `test` invocation.

### `phpscript fmt <path>...`

Format one or more PHP files or directories in place. A directory path formats
PHP files directly in that directory; append `/...` to include its
subdirectories. With no path, the command uses the current directory (`.`).

```bash
phpscript fmt script.php
phpscript fmt ./src        # PHP files directly in ./src
phpscript fmt ./src/...    # PHP files in ./src and its subdirectories
```

The formatter uses tabs for indentation, places class opening braces on the
next line, keeps function and control-statement braces on the same line, and
normalizes line endings to LF.

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
annotation, or `@startup` for a file the server runs before it listens. Files
that are only included by others have neither.

### `phpscript ast <file.php>`

Tokenize a PHP file and print its PHP-style token stream.

```bash
phpscript ast tests/fixtures/code/TemplateTest_phpscript.php
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
`public/`. PHP files in `public/` are executable by filename, and `/` resolves
to `public/index.php` when no annotated route handles it. Files outside
`public/` are never directly exposed.

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
