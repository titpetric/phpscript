# `phpscript list <path>...`

List routes, PHP files, and classes found beneath one or more paths as a Markdown table. A directory path lists PHP files directly in that directory. Append `/...` to include its subdirectories. A run without a path uses the current directory (`.`).

It also accepts the [global flags](README.md#global-flags): `-f`, `-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`. The flags below belong to this command.

```bash
phpscript list ./src        # PHP files directly in ./src
phpscript list ./src/...    # PHP files in ./src and its subdirectories
phpscript list index.php    # A specific PHP file
```

A `/...` sweep leaves out `.git`, `vendor` and `testdata` at any depth below the path: composer's install tree is somebody else's code, and `testdata` holds input a test feeds a tool, php a parser refuses among it. `phpscript list testdata/...` still lists a directory named outright, the way `go test` reads one. That is how `list`, `lint`, `fmt` and `info` read a path of PHP files. `phpscript test` collects `.phpt` suites through a separate walk, which skips `vendor` alone.

The Route column names the entry point a file declares: a `METHOD /path` annotation, `@startup` for a file the server runs before it listens, or `@schedule ...` for a clock or interval job. Files that are only included by others have neither.

`--bindings` reports on the binary in place of a tree: the functions, classes, methods and constants this build registers, as a script would type them. The command takes no path arguments. `--json` writes the rows as data. A generator renders them into [Go imports](../reference/extensions/go-imports.md). Each row holds fields `kind`, `name`, `signature`, `area`.

```bash
phpscript list --bindings                     # Everything the runtime binds
phpscript list --bindings | grep '| function' # Just the functions
phpscript list --bindings --runtime=go --json # The go surface, as data
```

The listing comes from the runtime itself: it reports what this build binds, read off the binary. Its counts agree with `phpscript info`. Two things are missing from the listing. Both need the binary's own Go source: the doc comment written next to each registration, and the PHP spelling of each parameter name. Signatures here hold reflected types, without parameter names, because reflection invents the names it reports (`$string1`, `$string2`). An invented name would read as the real one. [Implemented PHP APIs](../reference/extensions/implemented-apis.md) holds the annotated listing, written from the same reflection plus a scan of the source.

Each row of `--bindings` ends with the area that defines the name, a namespace such as `Runtime\PS\Core\Strings`. `--areas` lists the areas themselves: the `Runtime\PS` areas in install order, then the `Runtime\PX` overlay, and the opt-in `Runtime\Go` areas last, with the number of functions, classes and constants each defines and the number of names it registers over. Arguments narrow it to the namespaces named, and `--json` writes the rows as data. [Runtime areas](../reference/extensions/runtime-areas.md) describes the layout.

```bash
phpscript list --areas                  # Every area
phpscript list --areas 'Runtime\PX'     # The overlay
phpscript list --areas --json           # As data
```

`--runtime` selects the namespace the listing reports, and defaults to `runner.runtime` from the configuration. `--runtime=go` reports `Runtime\Go` alone: the builtins, the classes and constants, and all package areas an import can reach. No PHP throwable class answers, because the go runtime's errors are go's `error` and it has no exception class.

```bash
phpscript list --bindings --runtime=go          # The go surface
phpscript list --areas --runtime=go             # Runtime\Go and its packages
phpscript -f phpscript.yml list --bindings      # The runtime the file configures
```

`--classes` answers about a class what `go doc` answers about a Go type, in PHP spelling: the class docblock, then the properties the bound Go type holds, then the constructor and the method declarations. Every declaration ends at the `;` without a body: the implementation is Go. No PHP source exists for a reader to open. Without arguments it prints every class a script can construct. With arguments it prints those, using the spelling on the command line.

```bash
phpscript list --classes                  # Every class a script can construct
phpscript list --classes 'HTTP\Client'    # One class
phpscript list --classes Database Mail    # Several
```

Past the godoc, the description cites the Go type behind the binding and adds no further reference: `Binds <import path> package, uses <symbol>.`, with a pkg.go.dev link for a standard library symbol. A class whose bound Go type satisfies an interface the runtime or the standard library declares lists an `implements` clause naming the interface. A class satisfying no interface lists no clause.

`--classes --json` writes the same classes as data, for a generator that links each member in place of printing a block of PHP. Each entry holds both spellings of every symbol: the class name a script types and the Go type it binds. Each property holds a PHP name and its Go field name. Each method holds a PHP name and its Go method name. Each citation reports the Go symbol, plus the PHP name where the runtime honours one. Parameters come with their names and types, and the interface list with the pkg.go.dev link.

```bash
phpscript list --classes --json                   # Every class a script can construct, as data
phpscript list --classes --json 'HTTP\Mux' | jq .implements
```

The structure comes from reflection over this build. The prose and the parameter names come from an index over the Go source, embedded in the binary. This mode reads the index and `--bindings` does not. `atkins test:introspection` regenerates the index. The listing names the properties a script can reach: an unexported Go field resolves as absent. The exported ones remain, each with the snake_case spelling that property access resolves back to the Go field.

The other half of the question: which calls a tree makes that this build does not have. `phpscript lint` answers, reporting each `call to undefined function` or `new: undefined class` against the same registered set.
