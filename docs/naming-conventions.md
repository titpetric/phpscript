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
holds `regex.go` and `buffers.go`, `stdlib/core/` holds `strings.go`,
`arrays.go` and `shared_memory.go`. The pattern for a multi-file subject is
`<subject>_<aspect>.go`, sorted together by the subject. A package that covers
one subject drops the prefix, since the package name already carries it:
`stdlib/database/` holds `query.go` and `migrate.go`, not `database_query.go`,
and `stdlib/session/` holds `manager.go` and `storage_disk.go`.

In `stdlib/core` each of those files also carries its own `init()`, registering
only its own area. That is what keeps the files independent: an area is added,
moved or dropped without touching another file, and no file in the package
depends on another. The coercion they all need lives in `internal/phpval`
rather than in a shared file beside them, because a shared file is the coupling
the split exists to remove.

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
detail and does not appear: `Session\Manager` comes from a package called
`session` and `SharedMemory` from one called `core`, without either name
spelling its package, and `Database` kept its name when it moved to
`stdlib/database`.

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

### Documenting a binding

Every registration carries a doc comment, and the generated reference is
built from it. The comment sits directly above the `RegisterFunc` or
`RegisterConstructor` call, or on the named Go function it registers, godoc
style; the generator prefers the registration site, and rewrites a leading Go
symbol name in a godoc comment to the registered PHP name.

A single line reads `// symbol does this.` It starts with the name a script
types and ends with punctuation. A comment that needs more than one line is
either further `//` lines or a docblock, `/** ... */`:

```go
// strlen returns the length of $str in bytes.
rt.RegisterFunc("strlen", func(str string) int64 { return int64(len(str)) })
```

The published entry cites the comment together with the PHP signature derived
from the Go implementation, as a PHP code block:

```php
// strlen returns the length of $str in bytes.
function strlen(string $str): int
```

The comment describes what a script sees: the behaviour, the arguments as PHP
variables, and any divergence from PHP. Implementation notes (allocation
shapes, engine choices) stay in the godoc of the implementing function, with a
one-line registration-site comment published over them. Go parameters are
named after the PHP arguments, so the published signature reads as PHP does; a
name Go predeclares (`string`) takes a close variant (`str`), and a parameter
the runner fills as a by-reference setter is named after the PHP argument it
writes, which is how `preg_match` publishes `&$matches`.

### What is registered today

[`docs/reference/extensions/implemented-apis.md`](reference/extensions/implemented-apis.md)
is the inventory: every function and class the standard CLI runtime registers,
the Go package each binding lives in, its doc comment and its signature. It is
generated by `go run ./scripts/list-apis` from the live runtime and the source
tree, and `atkins test:introspection` regenerates it.

The names there are the convention applied, not an aspiration: each one is
what the fixtures and both demos type, `new Database("dbadmin")` and
`new Database\Migrate("dbadmin")` among them.

### Areas of interest

The standard library is much smaller than either Go's or PHP's, and there is no
plan for full coverage of either. These are the areas worth an implementation,
with the names they get when they arrive.

| Area          | PHP does it with | phpscript name                                       | State                                                      |
|---------------|------------------|------------------------------------------------------|------------------------------------------------------------|
| HTTP client   | `curl_*`         | `HTTP\Client`, `HTTP\Request`                        | Implemented                                                |
| JSON          | `json_encode`    | `json_encode`, `json_decode`                         | Implemented, PHP-compatible                                |
| YAML          | `yaml_*`         | `yaml_parse`, `yaml_emit`                            | Not implemented, aim for PHP-compatible                    |
| Time          | `DateTime`       | `DateTime`, `Time`, `Time\Duration`, `Time\Location` | Implemented as direct Go value bindings                    |
| Databases     | PDO              | `Database`                                           | Implemented                                                |
| Migrations    | none             | `Database\Migrate`                                   | Implemented                                                |
| Telemetry     | none             | `start_span`, later `Telemetry\Span`                 | Spans only                                                 |
| Sessions      | `$_SESSION`      | `Session\Manager`                                    | Implemented                                                |
| Shared memory | `shmop_*`, APCu  | `SharedMemory`                                       | Implemented                                                |
| Mail          | `mail`           | `mail`, `SMTP`                                       | Implemented; `mail` registers when the host binds a sender |
| Templating    | none             | native PHP                                           | minitpl runs as PHP source                                 |

An area not in this table is not in scope. Adding one is a decision about the
scope of the runtime, and goes through an issue before it goes through a
package.

## Getting from here to there

No registered name has to change; the migration outstanding is on the Go side.

**Move PHP's own library out of `stdlib/stdlib.go`.** The functions PHP defines
were spread across `stdlib/stdlib.go` and `stdlib/platform.go`. They split two
ways, and the line is behavioural rather than historical. `stdlib/compat` holds
the surface whose behaviour is the interpreter's: output buffering, `preg_*`,
the date functions, things defined by what the engine does rather than by what
they compute. `stdlib/core` holds the surface that computes: `strlen`,
`array_merge`, `json_encode`. Either way `stdlib` keeps only `Register`,
`RegisterFS` and the exception type.

The exception type stays because `Register` installs it before anything else,
and `stdlib` blank-imports `core`, so moving the registration there would make
a cycle.

Some of PHP's surface stays in `runner` rather than moving. `func_get_args`
returns the current frame's arguments, which only the runtime holds, so it is
installed on the scope in `runner/runtime.go` instead of being registered as a
shim. That is the test for staying: a function that cannot be written against
the public runtime API belongs in the runner. `compact` and `get_defined_vars`
also read the calling scope, but they reach it through
`runner.ScopeFromContext`, so they are ordinary registrations and move with the
rest.

`stdlib/core` is the catch-all: the areas of PHP's own library that compute,
plus the phpscript extensions each too small to be worth a package,
`SharedMemory`, `defer` and `register_shutdown_function`. An area that grows
past that gets its own package, as `stdlib/smtp`, `stdlib/span` and
`stdlib/http` did, as `Database` did when it moved to `stdlib/database`, and as
`Session\*` did when it moved to `stdlib/session`. Moving one costs nothing a
script can see, because the Go package a binding lives in is not part of what a
script types.

Renaming a registered class is the expensive change, which is why the rule is
applied before a binding lands rather than after. If one is ever needed, the new
name is registered first and the old one kept as a second registration of the
same constructor, with the removal a separate change after the fixtures and both
demos move.
