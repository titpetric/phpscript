# `phpscript list <path>...`

List routes, PHP files, and classes found under one or more paths as a Markdown table. A directory path lists PHP files directly in that directory; append `/...` to include its subdirectories. With no path, the command uses the current directory (`.`).

It also accepts the [global flags](README.md#global-flags): `-f`, `-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`. The flags below are this command's own.

```bash
phpscript list ./src        # PHP files directly in ./src
phpscript list ./src/...    # PHP files in ./src and its subdirectories
phpscript list index.php    # A specific PHP file
```

The Route column names the entry point a file declares: a `METHOD /path` annotation, `@startup` for a file the server runs before it listens, or `@schedule ...` for a clock or interval job. Files that are only included by others have neither.

`--bindings` reports on the binary in place of a tree: the functions, classes, methods and constants this build registers, as a script would type them. It takes no path arguments.

```bash
phpscript list --bindings                     # Everything the runtime binds
phpscript list --bindings | grep '| function' # Just the functions
```

The listing comes from the runtime itself: it reports what this build binds, not what a checked-in file says it binds; its counts agree with `phpscript info`. Two things are missing from it, and both need the binary's own Go source: the doc comment written next to each registration, and the PHP spelling of each parameter name. Signatures here carry reflected types and no parameter names, because reflection invents the names it reports (`$string1`, `$string2`), and they would read as the real ones. The annotated listing lives in [Implemented PHP APIs](../reference/extensions/implemented-apis.md), generated from the same reflection plus a scan of the source.

The other half of the question, what a tree calls that this build does not have, is `phpscript lint`, which reports every `call to undefined function` and `new: undefined class` against the same registered set.
