# How to collect Go code coverage from phpscript and phpx test

`phpscript test` runs PHP fixtures through a Go interpreter. The fixtures pass or fail, and they also exercise Go code. This guide builds a copy of the binary that records which Go statements the fixtures ran, runs the fixtures through it, and turns the result into a coverage profile.

The steps work for any Go program, which is why the examples name two binaries and nothing else: `phpscript` and `phpx`. Replace the name to repeat it.

## What you need

- Go 1.20 or later. Coverage for a binary arrived in 1.20, and everything below was run on 1.27.
- A checkout of the program. The examples start in its root.

## 1. Build the binary with coverage

A coverage build is an ordinary build with `-cover`:

```sh
go build -cover -o bin/phpscript-test .
```

For phpx, which is built with the `phpx` tag:

```sh
go build -cover -tags phpx -o bin/phpx-test .
```

Three flags decide what the build records.

| Flag | What it does |
|------|--------------|
| `-cover` | Instruments the program. Without it the other flags do nothing. |
| `-covermode=set\|count\|atomic` | `set` records whether a statement ran, `count` how many times, `atomic` counts safely across goroutines. Use `atomic` for an interpreter that runs fixtures in parallel. The default is `set`. |
| `-coverpkg=./...` | Names the packages to instrument. The default is the main module's packages. Spell it out so the scope is written down, and widen it to a list of import paths when you want a dependency in the profile. |

The usual form:

```sh
go build -cover -covermode=atomic -coverpkg=./... -o bin/phpscript-test .
```

### Why `go build` and not `go install`

`go install` accepts the same flags and writes to `$GOBIN`, but it has no `-o`. The binary takes its name from the package, so a coverage build of `phpscript` would overwrite the real one:

```sh
GOBIN=$PWD/bin go install -cover -covermode=atomic ./...   # writes bin/phpscript
```

When the copy must have its own name, build it and install the file:

```sh
go build -cover -covermode=atomic -coverpkg=./... -o bin/phpscript-test .
install -m 0755 bin/phpscript-test /usr/local/bin/phpscript-test
```

Do not set `-cover` in `GOFLAGS`. It would instrument every build the environment makes, including the one that produces the binary you ship.

## 2. Tell the binary where to write

An instrumented binary writes its data when it exits, into the directory named by `GOCOVERDIR`:

```sh
rm -rf cover/phpscript-test && mkdir -p cover/phpscript-test
GOCOVERDIR=cover/phpscript-test bin/phpscript-test test --matrix ./tests/...
ls cover/phpscript-test
```

The directory holds two kinds of file: `covmeta.<hash>`, which describes the program, and `covcounters.<hash>.<pid>.<time>`, one per process that ran. Run the binary again with the same variable and more counter files appear. Nothing is overwritten, so one directory can hold a whole pipeline of runs.

If the variable is not set the binary still runs and prints:

```
warning: GOCOVERDIR not set, no coverage data emitted
```

The directory must exist before the run. The runtime does not create it.

## 3. Turn the data into a profile

`go tool covdata` reads the directory:

```sh
go tool covdata percent  -i=cover/phpscript-test                     # one line per package
go tool covdata textfmt  -i=cover/phpscript-test -o cover/phpscript.cov
go tool covdata func     -i=cover/phpscript-test                     # per function
go tool covdata merge    -i=cover/a,cover/b -o cover/merged          # combine directories
```

`textfmt` writes the same text profile `go test -coverprofile` writes, so everything that reads one reads this:

```sh
go tool cover -func=cover/phpscript.cov | tail -1                    # total
go tool cover -html=cover/phpscript.cov -o cover/phpscript.html      # browsable report
```

## 4. Put it in a pipeline

One job builds, runs and converts. In atkins:

```yaml
  cover:test:
    desc: "Run the fixtures through the coverage build and write cover/phpscript.cov"
    cmds:
      - go build -cover -covermode=atomic -coverpkg=./... -o bin/phpscript-test .
      - rm -rf cover/run && mkdir -p cover/run
      - GOCOVERDIR=cover/run bin/phpscript-test test --matrix ./tests/...
      - go tool covdata textfmt -i=cover/run -o=cover/phpscript.cov
      - go tool covdata percent -i=cover/run
```

The same steps as a shell script:

```sh
#!/usr/bin/env bash
set -euo pipefail
go build -cover -covermode=atomic -coverpkg=./... -o bin/phpscript-test .
rm -rf cover/run && mkdir -p cover/run
GOCOVERDIR=cover/run bin/phpscript-test test --matrix ./tests/...
go tool covdata textfmt -i=cover/run -o cover/phpscript.cov
go tool cover -func=cover/phpscript.cov | tail -1
```

## 5. Run it as a tool of the real binary

phpscript finds executables named `<binary>-<tool>` on `PATH`, and a tool named `test` replaces the built-in command. With `phpscript-test` or `phpx-test` on `PATH`, the usual command line collects coverage:

```sh
PATH=$PWD/bin:$PATH phpscript test --matrix ./tests/...
```

phpscript does both details for you, in `cmd/phpscript/toolchain`:

- **The tool gets its arguments without its own name.** `phpx test --matrix x` reaches `phpx-test` as `--matrix x`. `toolchain.CommandArgs` reads the dash in `argv[0]` and puts the command first, so the binary sees `phpx-test test --matrix x`. A name whose tool part holds another dash, such as `phpscript-linux-amd64`, is a release file and is left alone.
- **`GOCOVERDIR` is read before `init` runs.** The coverage runtime looks at the variable as the process starts, so setting it from Go code is too late. `toolchain.CoverRelaunch` checks the build info for `-cover=true`, and when `GOCOVERDIR` is unset it sets it to `cover/<binary>` and starts the program again. An explicit `GOCOVERDIR` in the caller's environment wins.

```go
func main() {
	os.Args = toolchain.CommandArgs(os.Args)
	toolchain.CoverRelaunch()
	// ...
}
```

## 6. Check it worked

```sh
rm -rf cover/run && mkdir -p cover/run
GOCOVERDIR=cover/run bin/phpscript-test test --matrix ./tests/fixtures/strings/...
ls cover/run | wc -l                 # at least two files: one meta, one counters
go tool covdata percent -i=cover/run # a coverage line per instrumented package
```

If `percent` prints `no applicable files found in input directories`, the directory has no data: the variable was unset, the directory was wrong, or the process was killed before it exited. A process that ends through a signal does not flush its counters. `os.Exit` and a normal return both do.

## Reading the numbers

- Coverage of an interpreter measures the Go it ran, not the PHP it was given. A fixture suite that passes can still leave a Go package at 0%.
- Combine sources with `merge`. `go test -cover` binaries write to `GOCOVERDIR` too, when run with `-test.gocoverdir`, so unit tests and fixtures can share one report.
- Keep `cover/` out of version control. Both repositories ignore it.
