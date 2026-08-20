# Naming conventions

Two naming systems meet in this repository. Go code is laid out as packages
under the module root, and PHP code sees a set of function and class names
registered on a runtime. This document records what both look like today, the
rules a new addition follows, and the areas the standard library is expected to
grow into.

## Go package layout

### Root packages

The module root holds one package per responsibility. Each one is a stage of
the pipeline from PHP source to output, or a service the stages share.

| Package         | Responsibility                                                                     |
|-----------------|------------------------------------------------------------------------------------|
| `annotations`   | Discovers `@route`, `@startup` and `@schedule` comments and gives them a lifecycle |
| `cmd/phpscript` | One directory per CLI subcommand                                                   |
| `config`        | The `config/config.yml` configuration model                                        |
| `flatstack`     | The embedding API for the compile-once bytecode backend                            |
| `formatter`     | Prints an AST back to source                                                       |
| `internal`      | Code the module uses and nobody outside it can import                              |
| `lint`          | Diagnostics over PHP files, including flatstack support checks                     |
| `list`          | A markdown inventory of routes, files and classes                                  |
| `model`         | AST nodes, runtime values and class metadata shared by parser and runner           |
| `parser`        | PHP source to `model` AST                                                          |
| `runner`        | Tree-walking evaluation of a `model.Program`                                       |
| `stdlib`        | The functions and classes a script can call                                        |
| `telemetry`     | The only package that imports oida; everything else instruments through it         |
| `tests`         | The fixture harness behind `phpscript test`                                        |

`parser` and `runner` do not import each other; they meet in `model`. That
split is why `model` exists as a package rather than as types on either side.

`internal/` is where a package goes when it has no callers outside this module
and no API worth committing to. `internal/arrayi64` holds a generated integer
sort used by `stdlib`; the compiler enforces that nothing outside the module can
import it, so the package can change shape without a deprecation. Prefer it over
a root package for anything a host would never call.

### Adding a package

The root level is closed. A change that needs somewhere to live goes into an
existing package, or into a subdirectory of the package that owns the area:
`stdlib/<area>` for anything a script calls, `flatstack/engine` for the
bytecode backend, `cmd/phpscript/<command>` for a subcommand.

A new root package has to answer all of:

1. What existing package would otherwise hold this, and what breaks if it does?
2. Which packages import it? A package imported by exactly one other package is
   part of that package.
3. Is it a boundary to something outside the process (a wire protocol, a
   provider library, a database driver)? `telemetry` is a root package because
   it is the single import site for oida. That is the shape that earns one.
4. Does it name a responsibility, or a collection? `parser` is a
   responsibility. A package named for what it contains rather than what it
   does belongs somewhere else.

An addition that cannot answer these is an area of an existing package, and
gets a file there instead.

### Subcommands

Every entry in `cmd/phpscript/` is a package named after the subcommand, with
`run.go` exporting `NewCommand() *cli.Command` and a `Run` function holding the
work. `main.go` imports each one and registers the command it returns. The
subcommand package holds argument handling and output formatting only; the
behaviour lives in a root package it calls, which is why `phpscript lint` is a
few dozen lines over `lint`.

## Standard library layout

```
stdlib/                  the glue: Register, RegisterFS, the shared exception
stdlib/compat/<area>.go  PHP's own standard library, one file per area
stdlib/<area>/           extensions with no PHP counterpart, one package per area
```

`stdlib.Register` installs the shims defined in `stdlib` itself, then runs every
installer contributed by an imported binding package. A binding package
registers itself in `init`:

```go
func init() {
	runner.RegisterBinding(Register)
}

// Register installs symbols into the runtime.
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("start_span", telemetry.StartSpan)
}
```

`stdlib/imports.go` blank-imports the standard set, the way a program imports a
`database/sql` driver. A host that wants a different set builds its runtime
without `stdlib` and passes its own installers to `Register`.

Inside a package, a file is named for the area it covers: `stdlib/compat/`
holds `regex.go` and `buffers.go`, `stdlib/ps/` holds `session_storage_disk.go`
and `shared_memory.go`. The pattern for a multi-file subject is
`<subject>_<aspect>.go`, sorted together by the subject. A package that covers
one subject drops the prefix, since the package name already carries it:
`stdlib/database/` holds `query.go` and `migrate.go`, not `database_query.go`.

## PHP-visible names

A registered name is what a script types. It is a compatibility surface and
renaming it breaks scripts, so the rule is decided before the binding lands.

### The rule

**A name PHP already defines keeps PHP's spelling and behaviour.** `json_encode`,
`preg_match`, `ob_get_clean`, `Exception`. These live in `stdlib/compat` or in
`stdlib` itself. Where the implementation cannot match PHP, the difference is
documented rather than renamed around: `preg_*` compiles with RE2 or a
backtracking engine depending on the pattern, and is still called `preg_match`.

Taking PHP's name is a claim about behaviour, so it is settled by a `.phpt`
fixture whose expected output came from running the same source through `php`,
not from running it through phpscript. Until that diff is empty the binding
matches PHP only by intention. Where it cannot match, the fixture records what
a script does see and the `description` says why, which is what makes the
difference documented rather than latent. `docs/testing.md` has the procedure.

A binding with no PHP counterpart has no second implementation to compare
against, so its fixture is the definition of the behaviour rather than a check
on it. That is a reason to write the fixture carefully, not a reason to skip
it: `Storage`, `Database` and `SharedMemory` are only specified by the fixtures
that exercise them.

A PHP-named call still gets its own package when it needs one. `phpinfo` is
registered from `stdlib/info` because it reports on the runtime rather than
computing anything, and ships a PHP script beside the binding. The package is
where the code lives; the name a script types is unaffected by it.

**Everything else is a class named for the subject a script works with.**
`Database`, `Session\Manager`, `SharedMemory`, `SMTP`. The name says what is
being used, not what implements it: `Database` is the canonical database client
because a database is what a script has a handle to, and it is not renamed after
the Go package behind it or after the driver it connects through. There is no
project-wide prefix on a registered name. The Go package is an implementation
detail and does not appear: `stdlib/ps` holds `Session\*` and `SharedMemory`
without either of them spelling `ps`, and `Database` kept its name when it moved
to `stdlib/database`.

A second segment is for a family under the subject, and only when there is one.
`Session\Storage\Memory` and `Session\Storage\Disk` are two implementations of
one thing, so `Session\Storage` earns a segment; `Database\Migrate` is a
distinct object obtained from the same subject. A subject with a single class
stays flat, which is why `SharedMemory` is one word and not `Memory\Shared`.

A binding that is one call, with no object to hold, registers a function instead
of a class: `mail`, `defer`, `register_shutdown_function`, `start_span`. These
take PHP's naming style, lowercase with underscores, whether or not PHP defines
them.

Methods are the Go method spelled as PHP writes methods. Dispatch is
case-insensitive, so Go's `Query` is called as `$db->query()` and `Incr` as
`$shm->incr()`; a multi-word Go method takes underscores, as
`$span->set_attribute()` does.

The two cases are ordered: PHP compatibility comes first, so a call PHP already
has does not acquire a class because the Go side models it as one.

Namespaces are registered with escaped Go strings:
`rt.RegisterConstructor("Session\\Storage\\Disk", NewSessionStorageDisk)`.

### What is registered today

| PHP name                     | Go package    | What it is                                                          |
|------------------------------|---------------|---------------------------------------------------------------------|
| `Exception` and the SPL set  | `stdlib`      | PHP's exception hierarchy over one Go type                          |
| `Database`                   | `stdlib/ps`   | A connection from the host platform, queried through the pdo bridge |
| `Database\Migrate`           | `stdlib/ps`   | Loads a migration set and runs it                                   |
| `Session\Manager`            | `stdlib/ps`   | Starts, reads and validates a session                               |
| `Session\Storage\Memory`     | `stdlib/ps`   | Session storage backed by process memory                            |
| `Session\Storage\Disk`       | `stdlib/ps`   | Session storage backed by files                                     |
| `SharedMemory`               | `stdlib/ps`   | A key value store shared across requests                            |
| `SMTP`                       | `stdlib/smtp` | A configured sender, also behind `mail`                             |
| `mail`                       | `stdlib/smtp` | PHP's `mail`, sent through the bound sender                         |
| `defer`                      | `stdlib/ps`   | Runs a callback when the script ends                                |
| `register_shutdown_function` | `stdlib/ps`   | PHP's shutdown hook                                                 |
| `start_span`                 | `stdlib/span`      | Opens a telemetry span the script ends itself                       |
| `phpinfo`                    | `stdlib/info`      | Reports the runtime, as `phpscript info` runs it                    |
| `memory_get_usage`           | `stdlib/internals` | Reports request-scoped memory allocations in bytes                  |

This is the convention, not an aspiration: each of these names is what the
fixtures and both demos type, `new Database("dbadmin")` and
`new Database\Migrate("dbadmin")` among them.
`docs/reference/extensions/implemented-apis.md` lists the full set of functions
and classes and is regenerated by `atkins test:introspection`.

### Areas of interest

The standard library is much smaller than either Go's or PHP's, and there is no
plan for full coverage of either. These are the areas worth an implementation,
with the names they get when they arrive.

| Area          | PHP does it with | phpscript name                       | State                                   |
|---------------|------------------|--------------------------------------|-----------------------------------------|
| HTTP client   | `curl_*`         | `HTTP\Client`, `HTTP\Request`        | Not implemented                         |
| JSON          | `json_encode`    | `json_encode`, `json_decode`         | Implemented, PHP-compatible             |
| YAML          | `yaml_*`         | `yaml_parse`, `yaml_emit`            | Not implemented, aim for PHP-compatible |
| Databases     | PDO              | `Database`                           | Implemented                             |
| Migrations    | none             | `Database\Migrate`                   | Implemented                             |
| Telemetry     | none             | `start_span`, later `Telemetry\Span` | Spans only                              |
| Sessions      | `$_SESSION`      | `Session\Manager`                    | Implemented                             |
| Shared memory | `shmop_*`, APCu  | `SharedMemory`                       | Implemented                             |
| Mail          | `mail`           | `mail`, `SMTP`                       | Implemented                             |
| Templating    | none             | native PHP                           | minitpl runs as PHP source              |

An area not in this table is not in scope. Adding one is a decision about the
scope of the runtime, and goes through an issue before it goes through a
package.

## Getting from here to there

No registered name has to change; the migration outstanding is on the Go side.

**Move PHP's own library into `stdlib/compat`.** The functions PHP defines are
spread across `stdlib/stdlib.go` and `stdlib/platform.go`, with only output
buffering and `preg_*` moved into `compat` so far. The rest follows by area, one
file at a time, leaving `stdlib` holding `Register`, `RegisterFS` and the
exception type.

Some of PHP's surface stays in `runner` rather than moving. `func_get_args`
returns the current frame's arguments, which only the runtime holds, so it is
installed on the scope in `runner/runtime.go` instead of being registered as a
shim. That is the test for staying: a function that cannot be written against
the public runtime API belongs in the runner. `compact` and `get_defined_vars`
also read the calling scope, but they reach it through
`runner.ScopeFromContext`, so they are ordinary registrations and move with the
rest.

`stdlib/ps` is where the phpscript-specific extensions live: the bindings that
have no equivalent in PHP and are each too small to be worth a package. It holds
`Session\*` and `SharedMemory` today. An area that grows past that gets its own
package, as `stdlib/smtp` and `stdlib/span` did and as `Database` did when it
moved to `stdlib/database`. Moving one costs nothing a script can see, because
the Go package a binding lives in is not part of what a script types.

Renaming a registered class is the expensive change, which is why the rule is
applied before a binding lands rather than after. If one is ever needed, the new
name is registered first and the old one kept as a second registration of the
same constructor, with the removal a separate change after the fixtures and both
demos move.
