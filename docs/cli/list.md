# `phpscript list <path>...`

List routes, PHP files, and classes found under one or more paths as a Markdown table. A directory path lists PHP files directly in that directory; append `/...` to include its subdirectories. With no path, the command uses the current directory (`.`).

It also accepts the [global flags](README.md#global-flags): `-f`, `-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`. The flags below are this command's own.

```bash
phpscript list ./src        # PHP files directly in ./src
phpscript list ./src/...    # PHP files in ./src and its subdirectories
phpscript list index.php    # A specific PHP file
```

A `/...` sweep leaves out `.git`, `vendor` and `testdata` at every depth below the path: composer's install tree is somebody else's code, and `testdata` holds input a test feeds a tool, php a parser refuses among it. `phpscript list testdata/...` still lists the directory named outright, as `go test` reads one. The commands that expand a path of PHP files read it this way: `list`, `lint`, `fmt` and `info`. `phpscript test` collects `.phpt` suites through a separate walk, which skips `vendor` alone.

The Route column names the entry point a file declares: a `METHOD /path` annotation, `@startup` for a file the server runs before it listens, or `@schedule ...` for a clock or interval job. Files that are only included by others have neither.

`--bindings` reports on the binary in place of a tree: the functions, classes, methods and constants this build registers, as a script would type them. It takes no path arguments.

```bash
phpscript list --bindings                     # Everything the runtime binds
phpscript list --bindings | grep '| function' # Just the functions
```

The listing comes from the runtime itself: it reports what this build binds, not what a checked-in file says it binds; its counts agree with `phpscript info`. Two things are missing from it, and both need the binary's own Go source: the doc comment written next to each registration, and the PHP spelling of each parameter name. Signatures here hold reflected types and no parameter names, because reflection invents the names it reports (`$string1`, `$string2`). An invented name would read as the real one. [Implemented PHP APIs](../reference/extensions/implemented-apis.md) holds the annotated listing, written from the same reflection plus a scan of the source.

Each row of `--bindings` ends with the area that defines the name, a namespace such as `Runtime\PS\Core\Strings`. `--areas` lists the areas themselves: the `Runtime\PS` areas in install order, the `Runtime\PX` overlay after them, and the opt-in `Runtime\Go` areas last, with the number of functions, classes and constants each defines and the number of names it registers over. Arguments narrow it to the namespaces named, and `--json` writes the rows as data. [Runtime areas](../reference/extensions/runtime-areas.md) describes the layout.

```bash
phpscript list --areas                  # Every area
phpscript list --areas 'Runtime\PX'     # The overlay
phpscript list --areas --json           # As data
```

`--classes` answers about a class what `go doc` answers about a Go type, in PHP spelling: the class docblock, then the properties the bound Go type holds, then the constructor and the method declarations. Every declaration ends at the `;` with no body, because the implementation is Go and no PHP a reader could open. With no arguments it prints every class a script can construct; with arguments it prints those, under the spelling on the command line.

```bash
phpscript list --classes                  # Every class a script can construct
phpscript list --classes 'HTTP\Client'    # One class
phpscript list --classes Database Mail    # Several
```

Past the godoc, the description cites the Go type behind the binding and adds no further reference: `Binds <import path> package, uses <symbol>.`, with a pkg.go.dev link for a standard library symbol. A class whose bound Go type satisfies an interface the runtime or the standard library declares lists an `implements` clause naming it; a class satisfying no interface lists no clause.

`--classes --json` writes the same classes as data, for a generator that links each member in place of printing a block of PHP. Each entry holds both spellings of every symbol: the class name a script types and the Go type it binds, each property under its PHP name and its Go field name, each method under its PHP name and its Go method name, and each citation as the Go symbol plus the PHP name where the runtime honours one. The parameters come with their names and types, and the interface list with the pkg.go.dev link.

```bash
phpscript list --classes --json                   # Every class a script can construct, as data
phpscript list --classes --json 'HTTP\Mux' | jq .implements
```

The structure comes from reflection over this build. The prose and the parameter names come from an index over the Go source, embedded in the binary. This mode reads it and `--bindings` does not; `atkins test:introspection` regenerates it. The listing names the properties a script can reach and no others: an unexported Go field resolves as absent, leaving the exported ones, each under the snake_case spelling property access resolves back to the Go field.

The other half of the question, what a tree calls that this build does not have, is `phpscript lint`, which reports every `call to undefined function` and `new: undefined class` against the same registered set.
