# Resources

Where to read further, grouped by what you are looking for: the reference
documentation that answers a question about the runtime, the two applications the book
takes its code from, the analysis that decided what `titpetric/phpscript-common` contains,
the packages the application depends on, and the commands you run against a project.

## The reference documentation

Everything under `docs/` in this repository.

- [Documentation index](../README.md) - the map of the pages below, and what phpscript is
  before it is a web server.
- [Language reference](../reference/README.md) - the PHP surface this runtime implements,
  function by function, with the behaviour that differs from PHP written next to the
  function it differs in.
- [Design decisions](../design.md) - why there is no inheritance, no dynamic dispatch and
  no exception hierarchy, and what an interface is when it does not dispatch.
- [Configuration](../configuration.md) - every key of `config.yml`, its default and what
  it changes, including virtual hosts, writable paths and the execution limits.
- [CLI usage](../usage.md) - every command and every flag, plus the Docker image and how
  to embed the runtime in a Go program.
- [Testing phpscript](../testing.md) - `.phpt` fixture format, the Go test helpers, and
  running the same fixture through three runtimes.
- [Test fixtures](../test-fixtures.md) - the generated inventory of what the fixture suite
  covers, one table per folder.
- [Naming conventions](../naming-conventions.md) - how Go packages, standard library
  bindings and PHP-visible names are chosen, and how to move an existing name.
- [Telemetry](../telemetry.md) - the recorder, the five debug views, and what a PHP script
  can put on a trace itself.
- [Flat-stack runtime](../flatstack.md) - the experimental bytecode backend, its native
  subset and when it falls back.
- [Allocation performance](../allocation-performance.md) - the measured allocation work on
  the interpreter, binding by binding.

### The use-case walkthroughs

[`docs/use-cases/`](../use-cases/README.md) builds one thing per page, end to end.

- [Building an application](../use-cases/application.md) - the layout, the config file,
  routes, a database and a template, from an empty directory.
- [Usage of Go bindings](../use-cases/bindings.md) - how a Go type reaches PHP and what a
  script sees of it.
- [Error handling](../use-cases/error-handling.md) - throwing with a status, the error
  page convention, and which requests get a page rather than a status.
- [Serving static files](../use-cases/static-files.md) - the document root, index pages,
  `autoindex`, and what is never served.
- [Routing](../use-cases/routing.md) - `@route`, `$_PATH`, methods, and what a path
  pattern accepts.
- [Virtual hosting](../use-cases/virtual-hosting.md) - a two site server worked through,
  and what one domain cannot reach on the other.
- [Database bindings](../use-cases/database.md) - `Database`, queries, transactions,
  `Database\Migrate` and the schema layout it applies.
- [HTTP client bindings](../use-cases/http-client.md) - calling another service from PHP.
- [Shared memory bindings](../use-cases/shared-memory.md) - the process-wide store and
  what survives a request.
- [Templating](../use-cases/templating.md) - installing minitpl, the search paths and the
  compile cache.

## The worked applications

Both are in this repository and both are covered by tests.

- [`demos/example`](../../demos/example) - the small application
  [Building an application](../use-cases/application.md) builds: one table, one form, a
  template and a handful of routes.
- [`demos/common-phpscript`](../../demos/common-phpscript) - the complete application this
  book teaches. `bootstrap.php` is the composition root, `routes/` holds one file per
  endpoint, `src/` is the composer package `titpetric/phpscript-common`, `schema/` holds
  the migrations, `jobs/` holds the scheduled work and `migrate.php` is the `@startup`
  file.

## The analysis behind titpetric/phpscript-common

[`demos/common-report/`](../../demos/common-report) is the record of what a PHP 5-era
framework contained, what of it is worth keeping, and what the replacement looks like. The
use-case documents are numbered `01` to `24`; the entries below are the ones to start
from.

- [`01-scope.md`](../../demos/common-report/01-scope.md) - what `demos/common` is, why it
  does not run on this runtime, and what the analysis is meant to produce.
- [`proposal.md`](../../demos/common-report/proposal.md) - the package that replaces it:
  the components, the tables and the boundary between package and application.
- [`proposal-requirements.md`](../../demos/common-report/proposal-requirements.md) - the
  concerns that cut across more than one use case, each stated once, with a score for how
  much of the legacy source depends on it.

The annotation proposals are design work on the runtime rather than on the package. They
are written against file and line numbers in this tree, so each one says what would have
to change.

- [`proposal-annotation.md`](../../demos/common-report/proposal-annotation.md) - the index:
  the three annotations that exist today and the seam the rest would attach to.
- [`proposal-annotation-auth.md`](../../demos/common-report/proposal-annotation-auth.md) -
  `@auth` and `@permission`, so a routed file's guard is declared rather than written as
  its first statement.
- [`proposal-annotation-middleware.md`](../../demos/common-report/proposal-annotation-middleware.md) -
  `@middleware <name> [key=value ...]`, the general seam the specific tags are built on.
- [`proposal-annotation-limits.md`](../../demos/common-report/proposal-annotation-limits.md) -
  `@concurrency`, `@timeout` and `@body`, and which of the three can be enforced without an
  interpreter change.
- [`proposal-annotation-ratelimit.md`](../../demos/common-report/proposal-annotation-ratelimit.md) -
  `@ratelimit`, enforced in Go before a PHP runtime is built.
- [`proposal-annotation-cache.md`](../../demos/common-report/proposal-annotation-cache.md) -
  `@cache` for response caching, and the five conditions it is accepted under.
- [`proposal-annotation-route-module.md`](../../demos/common-report/proposal-annotation-route-module.md) -
  default values, constrained and tail parameters in `@route`, and the three defects behind
  them.

## The packages

- [titpetric/minitpl](https://github.com/titpetric/minitpl) - the template engine
  `MiniTPL\Template` comes from. It compiles a template to PHP under the directory
  `set_compile_location()` names and includes the result.
- [titpetric/pdo](https://github.com/titpetric/pdo) - the upstream of the `Database`
  binding. `stdlib/database` wraps its client, which is what resolves a
  `PLATFORM_DB_<NAME>` entry into a connection.
- [go-bridget/mig](https://github.com/go-bridget/mig) - the migration runner behind
  `Database\Migrate`. It records progress per statement index in a `migrations` table,
  which is why migration files are append only.
- [titpetric/platform](https://github.com/titpetric/platform) - the service the
  `server:` block configures. It owns the router, the module lifecycle and the tracing
  middleware.
- [titpetric/oida](https://github.com/titpetric/oida) - the recorder behind the
  `telemetry:` block and the debug front end it mounts.

## The commands

Full flags and examples are in [CLI usage](../usage.md).

| Command                    | What it does                                                                                        |
|----------------------------|-----------------------------------------------------------------------------------------------------|
| `phpscript run <file.php>` | Parses and executes one script with the CLI SAPI. The default when a path is given with no command. |
| `phpscript server [dir]`   | Serves an application root: static files, PHP entrypoints, `@route`, `@startup` and `@schedule`.    |
| `phpscript list <path>...` | Lists PHP files, their classes and the entry point each one provides, as a Markdown table.          |
| `phpscript lint <path>...` | Reports assignment in a conditional and chained assignment; a parse error fails the run.            |
| `phpscript fmt <path>...`  | Formats PHP in place, refusing to write a file whose output does not reparse identically.           |
| `phpscript test <path>...` | Runs `.phpt` fixtures, with `--matrix` to compare the two engines against the `php` binary.         |
| `phpscript info [path]`    | Prints the runtime the way `phpinfo()` does, or Markdown docs for the classes found under a path.   |
| `phpscript ast <file.php>` | Prints the `token_get_all()` token stream of a file, for debugging the parser.                      |

A directory argument covers the PHP files directly in it; append `/...` to walk the tree
below it.

Next: [Creating phpscript applications](creating-phpscript-applications.md) is the front
matter, with the table of contents and the task index.
