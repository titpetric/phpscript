# Naming conventions

Two naming systems meet in this repository. Go packages are below the module root. PHP code sees function and class names registered on a runtime. This document records what both look like today, the rules a new addition follows, and the areas worth growing into.

## Go package layout

### Root packages

The module root holds one package per responsibility. Each package is a pipeline stage from PHP source to output, or a service the stages share.

| Package         | Responsibility                                                                       |
|-----------------|--------------------------------------------------------------------------------------|
| `annotations`   | Discovers `@route`, `@startup` and `@schedule` comments and runs them on a lifecycle |
| `cmd/phpscript` | One directory per CLI subcommand                                                     |
| `config`        | The `config/config.yml` configuration model                                          |
| `flatstack`     | The embedding API for the compile-once bytecode backend                              |
| `formatter`     | Prints an AST back to source                                                         |
| `internal`      | Code the module uses and no other module can import                                  |
| `lint`          | Diagnostics over PHP files, including flatstack support checks                       |
| `list`          | A markdown inventory of routes, files and classes                                    |
| `model`         | AST nodes, runtime values and class metadata shared by parser and runner             |
| `modelutil`     | The package-level functions over `model`: readers, walkers, conversions, contracts   |
| `parser`        | PHP source to `model` AST                                                            |
| `runner`        | Tree-walking evaluation of a `model.Program`                                         |
| `stdlib`        | The functions and classes a script can call                                          |
| `telemetry`     | The only package that imports oida; everything else instruments through it           |
| `tests`         | The fixture harness behind `phpscript test`                                          |

`parser` and `runner` do not import each other, meeting in `model`. That split is why `model` is a separate package, with the types on neither side.

`model` holds the types: a struct, its methods, plus a `Valid() error` stating the rule a filled value holds to. `modelutil` holds the functions over those types and declares no type: the readers over an `any` (`RangeValues`, `LenValues`, `IsCollection`, and `Keys`, `Values`, `Map`, `Typed[T]`, `Seq` above those), plus the conversions (`ToArray`, `CopyValue`, `GoWiden`), the walkers (`AssignsTo`, `RootName`, `UnwrapParenthesized`) and the whole-tree contract checks (`CheckInterfaces`, `ParseRoutePath`). A new function on a `model` type goes there, a new type in `model`. The one-way import keeps each file short: a reader opening `model/array.go` reads the array alone, not a walk of every shape a binding might return.

### The validation a type declares

`Valid() error` is the structural check a type makes of itself, for callers that fill a value without reading the grammar: a decoded request, a generated registration, a hand-written table. `model/valid.go` holds them for the declaration types. `model/route.go` and `model/gowidth.go` hold one check apiece, for the type each declares.

Each reports the first fault it finds and names the declaration. None reads a body, resolves a name in a program or reaches another declaration: the whole-tree checks are `modelutil`'s. `CheckInterfaces` shows the shape.

A package goes in `internal/` when no caller outside the phpscript module uses it. The API must not be worth committing to. `internal/arrayi64` holds a generated integer sort used by `stdlib`. The compiler stops imports from outside the module. The package can change shape without a deprecation. Prefer it to a root package for anything a host would never call.

### Adding a package

The root level stays closed. A change that needs somewhere to live goes into an existing package, or into a subdirectory of the package that owns the area: `stdlib/ps/<area>` for anything a script calls, `flatstack/engine` for the bytecode backend, `cmd/phpscript/<command>` for a subcommand.

A new root package answers four questions:

1. Which existing package would hold this? What breaks if it does?
2. Which packages import it? Exactly one importer makes a package part of that importer.
3. Does it bound something outside the process (a wire protocol, a provider library, a database driver)? `telemetry` is a root package: no other package imports oida. That shape earns a root package.
4. Does it name a responsibility, or a collection? `parser` is a responsibility. A package named after its contents belongs elsewhere.

An addition that cannot answer these is an area of an existing package, and gets a file there instead.

### Tools the pipeline runs

A tool that checks or generates this repository, with no script or host calling it, is a `main` package under `internal/cmd/<tool>`, run as `go run ./internal/cmd/<tool>`. The first is `internal/cmd/bindlint`: it reads the Go source for the binding rules [the allocation rules](allocation-performance.md) state. `atkins lint` executes that tool. No code outside the phpscript module imports a tool of this repository. The directory is `internal/` for that reason. The name `cmd` marks a `main` package: among packages of behaviour it would compile into nothing a caller can use.

`scripts/` holds the generators instead, including `scripts/list-apis`. New generators go there too. Output separates the two: a gate answers with an exit code, a generator writes a file.

### Subcommands

Every entry in `cmd/phpscript/` is a package named after the subcommand. Its `run.go` exports `NewCommand() *cli.Command` and holds a `Run` function. `main.go` imports every subcommand package and registers the command it returns. The subcommand package holds argument handling and output formatting only, while the behaviour belongs to a root package it calls. `phpscript lint` is a few dozen lines over `lint`.

## Standard library layout

```
stdlib/                  the glue: Register, RegisterFS, the shared exception
stdlib/ps/compat/<area>.go  PHP's own standard library, one file per area
stdlib/<area>/           extensions with no PHP counterpart, one package per area
```

`stdlib.Register` installs the shims defined in `stdlib` itself, then runs every installer contributed by an imported binding package. A binding package registers in `init`, at the area its folder maps to:

```go
func init() {
	runner.RegisterArea(`Runtime\PS\Span`, Register)
}

// Register installs symbols into the runtime.
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("start_span", telemetry.StartSpan)
}
```

`stdlib/imports.go` blank-imports the standard set, the way a program imports a `database/sql` driver. A host needing another set builds a runtime without `stdlib` and passes its installers to `Register`.

The area name is a namespace path: `stdlib/ps/<folder>` is `Runtime\PS\<Folder>`, a file of `stdlib/ps/core` is `Runtime\PS\Core\<File>`, and `stdlib/px/<folder>` is `Runtime\PX\<Folder>`. The runtime installs all `Runtime\PS` areas before any `Runtime\PX` area. [Runtime areas](reference/extensions/runtime-areas.md) lists the mapping, the `runner.areas` configuration and the opt-in `Runtime\Go` areas under `stdlib/go/<pkg>`.

A binding that reaches outside the runtime's sandbox - a process, where the filesystem root and `writable_paths` cannot follow - registers in a separate profile area instead, through `runner.RegisterProfileArea(`Runtime\PS\Exec`, runner.ProfileExec, Register)`. `stdlib/ps/pexec` is the example. `stdlib.Register` installs every area, and `stdlib.Mount(rt, stdlib.Secure|stdlib.Exec)` installs the areas the bitmask names. A host serving untrusted scripts mounts `stdlib.Default` (the secure surface) and spells every insecure area it grants at the call site. A name outside the mounted profile stays undefined. `function_exists` answers false. A call, a callable and `call_user_func` reach nothing, on both engines.

Inside a package, a file takes its name from the area it covers: `stdlib/ps/compat/` holds `regex.go` and `buffers.go`, `stdlib/ps/core/` holds `strings.go`, `arrays.go` and `shared_memory.go`. The pattern for a multi-file subject is `<subject>_<aspect>.go`, sorted together by the subject. A package that covers one subject drops the prefix: `stdlib/ps/database/` holds `query.go` and `migrate.go`, not the `database_query.go` form, and `stdlib/ps/session/` holds `manager.go` and `storage_disk.go`.

In `stdlib/ps/core` each file also holds an `init()`, registering that area alone. The files are independent: adding, moving or dropping an area touches no other file. No file in the package depends on another. `internal/phpval` holds the coercion they all need, since a shared file beside the package is the coupling the split removes.

`stdlib/shared` is the sole exception, and not for sharing between stdlib packages. `stdlib` imports `runner`. The reverse import does not exist. A behaviour both perform belongs to neither package: the form decoder builds `$_GET` in the runner and answers `parse_str()` in a script, where both must agree. That package imports `model` and `internal/phpval` alone, which keeps it importable from both sides. A helper only stdlib needs belongs in `internal/phpval`, and one only the runner needs belongs in `runner`.

## PHP-visible names

A registered name is the text a script types. The name is a compatibility surface, and renaming it breaks scripts. Settle the decision before the binding lands.

### The rule

**A name PHP already defines keeps PHP's spelling and behaviour.** `json_encode`, `preg_match`, `ob_get_clean`, `Exception`. These go in `stdlib/ps/compat` or in `stdlib` itself. Where the implementation cannot match PHP, the reference records the difference under PHP's name: `preg_*` compiles with RE2 or a backtracking engine depending on the pattern, and is still called `preg_match`.

Taking PHP's name claims behaviour, settled by a `.phpt` fixture. Its expected output comes from running the same source through `php`, not through phpscript. Until that diff is empty the binding matches PHP only by intention. Where it cannot match, the fixture records the output a script sees. The `description` states why and puts the difference on the record. `docs/testing.md` has the procedure.

A binding with no PHP counterpart to compare against leaves the fixture to define its behaviour. Write that fixture carefully. `Storage`, `Database` and `SharedMemory` have no specification except the fixtures that exercise them.

A PHP-named call still gets a package when it needs one. `stdlib/ps/info` registers `phpinfo` because that call reports on the runtime and computes nothing, while the package ships a PHP script next to the binding. The package houses the code, while the name a script types does not change with it.

**Everything else is a class named for the subject a script works with.** `Database`, `Session\Manager`, `SharedMemory`, `Mail`. The name states the subject a script uses. Implementation stays hidden. `Database` is the canonical client: a script's handle reaches a database through the Go package and driver behind that name. No project-wide prefix applies to a registered name. The Go package is an implementation detail and does not appear: `Session\Manager` comes from a package called `session` and `SharedMemory` from one called `core`, without either name spelling its package, and `Database` kept its name when it moved to `stdlib/ps/database`.

A second segment names a family beneath a subject, only when one exists. `Session\Storage\Memory` and `Session\Storage\Disk` are implementations of one subject, and `Session\Storage` earns a segment. `Database\Migrate` is a distinct object from the same subject. A subject with a single class stays flat. `SharedMemory` takes one word, not `Memory\Shared`.

A binding of a single call, holding no object, registers a function, not a class: `mail`, `defer`, `register_shutdown_function`, `start_span`. These take PHP's naming style, lowercase with underscores, whether PHP defines them or not.

Methods are the Go method spelled as PHP writes methods. Dispatch is case-insensitive. Go's `Query` reaches a script as `$db->query()`. Go's `Incr` reaches it as `$shm->incr()`. A multi-word Go method takes underscores, like `$span->set_attribute()`.

The two cases have an order: PHP compatibility comes first. A call PHP already has gets no class, even when the Go side models a class.

Registering a namespace takes escaped Go strings: `rt.RegisterConstructor("Session\\Storage\\Disk", NewSessionStorageDisk)`.

### Documenting a binding

Every registration takes a doc comment, from which the generated reference comes. It sits directly above the `RegisterFunc` or `RegisterConstructor` call, or on the named Go function it registers, godoc style. The generator prefers the registration site, and rewrites a leading Go symbol name in a godoc comment to the registered PHP name.

A single line reads `// symbol does this.` It starts with the name a script types and ends with punctuation. A comment that needs more than one line is either further `//` lines or a docblock, `/** ... */`:

```go
// strlen returns the length of $str in bytes.
rt.RegisterFunc("strlen", func(str string) int64 { return int64(len(str)) })
```

The published entry cites the comment together with the PHP signature derived from the Go implementation, as a PHP code block:

```php
// strlen returns the length of $str in bytes.
function strlen(string $str): int
```

The comment describes the behaviour a script sees: the arguments as PHP variables, and any divergence from PHP. Implementation notes (allocation shapes, engine choices) stay in the godoc of the implementing function. The generator publishes a one-line registration-site comment over them. Go parameters take the names of the PHP arguments and the published signature reads as PHP does, while a name Go predeclares (`string`) takes a close variant (`str`). A by-reference setter parameter takes the name of the PHP argument it writes, and `preg_match` publishes `&$matches`.

### What the runtime registers today

[`docs/reference/extensions/implemented-apis.md`](reference/extensions/implemented-apis.md) is the inventory: every function and class the standard CLI runtime registers, the Go package of each binding, its doc comment and its signature. `go run ./scripts/list-apis` writes it from the live runtime and the source tree, and `atkins test:introspection` regenerates it.

The names there record the convention in use. The fixtures and the demos both type every name listed, `new Database("dbadmin")` and `new Database\Migrate("dbadmin")` among them.

### Areas of interest

The standard library is much smaller than Go's or PHP's. There is no plan for full coverage of either. These are the areas worth an implementation, with the names they get when they arrive.

| Area          | PHP does it with | phpscript name                                       | State                                                                                                  |
|---------------|------------------|------------------------------------------------------|--------------------------------------------------------------------------------------------------------|
| HTTP client   | `curl_*`         | `HTTP\Client`, `HTTP\Request`                        | Implemented                                                                                            |
| JSON          | `json_encode`    | `json_encode`, `json_decode`                         | Implemented, PHP-compatible                                                                            |
| YAML          | `yaml_*`         | `yaml_parse`, `yaml_emit`                            | Not implemented, aim for PHP-compatible                                                                |
| Time          | `DateTime`       | `DateTime`, `Time`, `Time\Duration`, `Time\Location` | Implemented as direct Go value bindings                                                                |
| Databases     | PDO              | `Database`                                           | Implemented                                                                                            |
| Migrations    | none             | `Database\Migrate`                                   | Implemented                                                                                            |
| Telemetry     | none             | `start_span`, later `Telemetry\Span`                 | Spans only                                                                                             |
| Sessions      | `$_SESSION`      | `Session\Manager`                                    | Implemented                                                                                            |
| Shared memory | `shmop_*`, APCu  | `SharedMemory`                                       | Implemented                                                                                            |
| Mail          | `mail`           | `mail`, `Mail`                                       | Implemented; both resolve a host-configured server by name                                             |
| Templating    | none             | native PHP                                           | minitpl runs as PHP source                                                                             |
| Processes     | `exec`, `system` | `exec`, `system`, `passthru`, `shell_exec`           | Implemented; `proc_*` is not, and needs a handle type                                                  |
| Expressions   | `preg_*`         | `preg_*`, `Regexp\Compile`, `Regexp\CompilePOSIX`    | Both implemented: PCRE under PHP's names, RE2 under its own. See [Regexp bindings](bindings-regexp.md) |

An area not in this table is not in scope. Adding one is a decision about the scope of the runtime, and an issue settles it before anyone writes a package.

## Getting from here to there

No registered name changes. The migration outstanding is on the Go side.

**Move PHP's own library out of `stdlib/stdlib.go`.** The functions PHP defines split two ways on a behavioural line. `stdlib/ps/compat` holds the surface whose behaviour is the interpreter's: output buffering, `preg_*`, the date functions, each defined by the engine, not by its computation. `stdlib/ps/core` holds the surface that computes: `strlen`, `array_merge`, `json_encode`. Either way `stdlib` keeps only `Register`, `RegisterFS` and the exception type.

The exception type stays because `Register` installs it before anything else, and `stdlib` blank-imports `core` and moving the registration there would make a cycle.

Some of PHP's surface stays in `runner`. `func_get_args` returns the current frame's arguments, which only the runtime holds. `runner/runtime.go` installs it on the scope, never registering it as a shim. That is the test for staying: a function nobody can write against the public runtime API belongs in the runner. `compact` and `get_defined_vars` also read the calling scope, but they reach it via `runner.ScopeFromContext`. They are ordinary registrations that move with the rest.

`stdlib/ps/core` is the catch-all: the areas of PHP's own library that compute, plus the phpscript extensions each too small to be worth a package, `SharedMemory`, `defer` and `register_shutdown_function`. An area that grows past that gets a package: `stdlib/ps/mail`, `stdlib/ps/span` and `stdlib/ps/http`, `Database` when it moved to `stdlib/ps/database`, and `Session\*` when it moved to `stdlib/ps/session`. Moving one costs nothing a script can see, because the Go package of a binding is not part of the name a script types.

Renaming a registered class is the expensive change, hence the rule applies before a binding lands. If one is ever needed, the change registers the new name first and keeps the old as a second registration of the same constructor. Removal is a separate change, after the fixtures and the demos move.
