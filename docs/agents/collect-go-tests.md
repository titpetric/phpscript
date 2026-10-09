# Collect: go test

Pass/fail and per-package duration from `go test`, including the tests that gate the build on an allocation count.

Read this when the artifact needed is a test run. It runs under the contract in [README.md](README.md); that document is not restated here.

## What this collects

Pass/fail per package and per test, and the wall-clock duration of each package.

Not per-operation cost, and not percentiles. A duration here is one sample.

## Preconditions

The contract's lock and `CGO_ENABLED=0`. A correctness run takes the lock too: it holds every core it is given for the length of the suite.

The database fixtures read their connection strings from `.env.testing`:

```sh
set -a; export $(grep -v ^# .env.testing | xargs -d "\n"); set +a
```

Exported rather than sourced. `. ./.env.testing` does not work: three of the four values carry an unquoted `&` and the fourth carries unquoted parentheses, so bash backgrounds three assignments and takes the fourth as a syntax error. atkins reads the same file with a dotenv parser, so `env: include: .env.testing` works where the shell does not. The file is left as it is.

The sqlite entries are in-memory and need nothing. The mysql and postgres entries need the compose services, which `atkins db:up` starts. A package that fails for a missing database is not a regression and is reported as a missing precondition.

`-count 1` so the test cache answers from the run rather than from a previous one.

## The run

The whole tree, with the machine-readable record:

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  set -a; export $(grep -v ^# .env.testing | xargs -d "\n"); set +a
  CGO_ENABLED=0 gotestsum --jsonfile bench-gotest-after.json \
    -- -count 1 ./...
'
```

With coverage, which is the form `atkins cover` reads from:

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  set -a; export $(grep -v ^# .env.testing | xargs -d "\n"); set +a
  mkdir -p cover
  CGO_ENABLED=0 gotestsum -- -count 1 -cover -coverpkg=./... \
    -coverprofile=cover/pkg.cov ./...
'
```

The package list goes last. `gotestsum` hands everything after `--` to `go test`, and `-coverprofile` writes nothing without complaining when the list precedes the flags.

Under the race detector, over the two packages that hold concurrent state:

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  CGO_ENABLED=1 go test -race -count 1 ./flatstack/... ./runner/...
'
```

`-race` requires cgo, which is the one place the `CGO_ENABLED=0` rule is set aside. A race run produces no number and is never a timing baseline.

One package:

```sh
CGO_ENABLED=0 go test -count 1 -v ./runner
```

## The artifact

`bench-gotest-<side>.json`, which is test2json: one JSON object per line, with `Action` (`run`, `pass`, `fail`, `output`), `Package`, `Test` and `Elapsed`. Already ignored by `bench*.json`.

`cover/pkg.cov` when `-cover` was on. Already ignored by `/cover/*` and `*.cov`. It is large, in the tens of megabytes, and it stays behind for `go tool cover -html`.

`bench-manifest-<side>.txt` alongside, as the contract requires.

## Reading it

The ranking:

```sh
gotestsum tool slowest --jsonfile bench-gotest-after.json
```

`tests/runner` dominates the suite and its share is sleeps, not work: the execution-limit and client-abort tests drive `set_time_limit`, `usleep` and `connection_aborted` against the real clock. Report it as excluded rather than as the slowest package, or the reading points at a sleep.

`tests` is the next largest and runs the whole fixture corpus twice, once per engine. `annotations`, `stdlib/internals` and `stdlib/http` carry real tickers, sleeps and listeners. `cmd/phpscript/test` shells out to the `php` binary.

Three tests fail rather than skip when they regress, and a failure in one is a finding and not a flake:

| Test                                       | Where                          | What a failure means                                                                                     |
|--------------------------------------------|--------------------------------|----------------------------------------------------------------------------------------------------------|
| `TestFlatstackPrecompiledAllocationBudget` | `tests/flatstack/load_test.go` | A precompiled program allocates more per run than its budget. The budget is never raised to make it pass |
| `TestLexOperatorNoAlloc`                   | `parser/lexer_test.go`         | The lexer's operator path started allocating                                                             |
| `TestNoInheritanceAtRuntime`               | `runner`                       | `model.Class` regrew a `Parent` field. A design guard, not a budget                                      |

The contract's test-immutability rule covers what may and may not be done about any of them.

Several packages log database connection lines at init. It is noise, not a failure.

## What this cannot answer

| Question                      | Document                                                                                            |
|-------------------------------|-----------------------------------------------------------------------------------------------------|
| What one operation costs      | [collect-go-benchmarks.md](collect-go-benchmarks.md)                                                |
| p50, p95 or p99               | [collect-phpscript-test.md](collect-phpscript-test.md)                                              |
| Which function the time is in | [collect-phpscript-run.md](collect-phpscript-run.md), and [profile.md](profile.md) for the layering |
