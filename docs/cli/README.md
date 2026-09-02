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
[Configuration](../configuration.md) for every available setting.

`phpscript --help` prints the whole document: the commands, the global flags,
each command's own flags and a table of worked examples per command. On a
terminal the tables are drawn; redirected, the same document comes out as
markdown.

## Global flags

Every command accepts these, and a command reads the ones it has a use for.
They may be written before the command name or after it, so
`phpscript --cover server` and `phpscript server --cover` are the same run.

| Flag              | What it does                                                                                                             |
|-------------------|--------------------------------------------------------------------------------------------------------------------------|
| `-f`, `--file`    | Read a configuration file over the built-in defaults.                                                                    |
| `-w`, `--workdir` | Change to this directory first, so every relative path resolves below it. The configuration file is read from there too. |
| `--include`       | Include a PHP file ahead of every entrypoint, when it exists. Also the `runner.include` configuration key.               |
| `-v`, `--verbose` | Report more. What that means is the command's: fixture failures, bound names, a coverage percentage.                     |
| `--cpuprofile`    | Write a pprof CPU profile of the command.                                                                                |
| `--memprofile`    | Write a pprof heap profile when the command ends.                                                                        |
| `--cover`         | Measure statement coverage. `line` writes the profile, `func` and `file` also print a report.                            |
| `--coverfile`     | Where the profile goes. Implies `--cover`; `{time}` expands to a UTC timestamp and missing directories are created.      |

`--include` is what makes one setting cover every way a tree is executed: the
server includes it ahead of each request's entrypoint, `run` ahead of the
script, `test` ahead of each fixture and `lint` ahead of the checks, so the
names a linter knows are the names a request will find. A composer autoloader
is the usual file:

```bash
phpscript --include vendor/autoload.php server
phpscript --include vendor/autoload.php lint ./...
```

A tree that always wants it sets `runner.include` in its configuration instead.
A virtual host sets its own in the `runner` block of its `phpscript.yml`,
because each site has its own vendor directory; `--include` is what the
operator sets for a tenant that named none.

## Commands

One page per command. `phpscript --help` prints the same set as one document,
with a table of worked examples per command.

| Command                                     | What it does                                                                 |
|---------------------------------------------|------------------------------------------------------------------------------|
| [`phpscript run <file.php>`](run.md)        | Parse and execute a PHP script in CLI mode. The default command.             |
| [`phpscript server [directory]`](server.md) | Serve one application root, or the virtual hosts the configuration lists.    |
| [`phpscript test <path>...`](test.md)       | Run the `.phpt` fixtures below a path, on one runtime or on all three.       |
| [`phpscript lint <path>...`](lint.md)       | Report undefined names, unreachable code and flatstack compatibility.        |
| [`phpscript fmt <path>...`](fmt.md)         | Rewrite PHP files in the canonical form, or name the ones that would change. |
| [`phpscript list <path>...`](list.md)       | A markdown inventory of the routes, files and classes of a tree.             |
| [`phpscript info [path...]`](info.md)       | The runtime environment, and everything this build binds.                    |
| [`phpscript ast <file.php>`](ast.md)        | The token stream a file parses to.                                           |
| [`phpscript version`](version.md)           | Build and module information.                                                |

## Docker image

Build a local binary and image from source:

```bash
CGO_ENABLED=0 go build -o bin/ .
docker build -t titpetric/phpscript:latest -f docker/Dockerfile .
```

See [compose.yml](../../compose.yml) for the development/test services used by
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
