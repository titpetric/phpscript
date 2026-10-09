# Collect: go test -bench

`ns/op`, `B/op` and `allocs/op` from the Go benchmarks, as benchstat input.

Read this when the artifact needed is a benchmark sweep. It runs under the contract in [README.md](README.md); that document is not restated here.

## What this collects

Three numbers per benchmark, and the benchstat comparison between two sets of them.

No percentiles: a Go benchmark reports a mean over its iterations. No custom metrics, because there is no `b.ReportMetric` anywhere in the tree.

## Preconditions

The contract's lock, private binary and `CGO_ENABLED=0`. A sweep takes minutes of pinned CPU and is out of the default pipeline.

Nothing else may run on the box. The lock is what enforces that against other agents; the operator enforces it against everything else.

## The run

`-run XXX` matches no test, so the sweep runs benchmarks only. Without it every test runs first and its time lands in the same wall clock.

The pinned job, which is every serial benchmark:

```sh
PARALLEL='BenchmarkLookup$|BenchmarkLookupHandler|BenchmarkFlatstackParallelHostBridge'

flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  CGO_ENABLED=0 taskset -c 3 nice -n -20 go test \
    -run XXX -bench . -skip "'"$PARALLEL"'" \
    -benchmem -benchtime 1s -count 6 ./... | tee bench-go-after.txt
'
```

The unpinned job, over the three parallel benchmarks:

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  CGO_ENABLED=0 nice -n -20 go test -p 1 \
    -run XXX -bench "'"$PARALLEL"'" \
    -benchmem -benchtime 1s -count 10 \
    ./tests/runner ./tests/flatstack | tee -a bench-go-after.txt
'
```

One benchmark, while iterating on a change:

```sh
CGO_ENABLED=0 taskset -c 3 nice -n -20 go test \
  -run XXX -bench '^BenchmarkEngineExprHeavy$' -benchmem -count 6 ./tests/flatstack
```

`-benchtime 1s`, not a fixed iteration count, so a benchmark that got faster takes more iterations and keeps its sample size.

The binding families are the exception. `../allocation-performance.md` publishes their `ns/op` to the nanosecond and sweeps them at a fixed `-benchtime 200000x`, to hold the iteration count still across a shape change. A `1s` sweep of the same rows comes back an order of magnitude wider than the differences that document reports, so a sweep that contradicts its table has not found a regression - it has measured something else. Re-measure those rows the way that document does before reading anything into them.

## The artifact

`bench-go-<side>.txt`, raw `go test -bench` output including the `goos`, `goarch`, `pkg` and `cpu` header lines. Already ignored by `bench*.txt`.

The comparison, which is the thing quoted in a pull request:

```sh
benchstat bench-go-before.txt bench-go-after.txt
```

For the engine pair, which is named for the column split:

```sh
benchstat -col /engine bench-go-after.txt
```

`bench-manifest-<side>.txt` alongside, as the contract requires. For the unpinned job it records `GOMAXPROCS`, because that job does not pin it.

## Reading it

Ninety-four benchmarks in eighteen files across ten packages. What each file prices:

| File                                                         | What it prices                                                                                                                                                                                                               |
|--------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `model/value_test.go`                                        | `model.Array` construction, each case paired against a legacy array. List mode wins where the array stays a list and costs a little where a string key promotes it, so read the pair; neither direction holds for both       |
| `parser/parser_test.go`, `parser/tokenizer_test.go`          | Parse and tokenize throughput, with `b.SetBytes`                                                                                                                                                                             |
| `runner/expr_cache_test.go`                                  | The expression engine, one benchmark per node shape, after every cache layer is warm                                                                                                                                         |
| `stdlib/core/*_test.go`, `stdlib/stdlib_test.go`             | Individual stdlib bindings, hoisted against legacy forms                                                                                                                                                                     |
| `tests/bindings_test.go`                                     | The binding surface: constructors, the reflection call path, and end-to-end scripts. The numbers in `../allocation-performance.md` come from here                                                                            |
| `tests/expr_bench_test.go`, `tests/singleshot_bench_test.go` | Expression-heavy scripts end to end, and a cold-compile fixture run                                                                                                                                                          |
| `tests/request_bench_test.go`                                | The request cycle, traced and untraced                                                                                                                                                                                       |
| `tests/fixture_test.go`                                      | One HTTP handler on three engines, `go_handler` being the native control the two PHP arms are read against. That control is the least stable row in its own comparison, so a claim about the gap needs its spread quoted too |
| `tests/runner/*_bench_test.go`                               | Symbol lookup against tree size, fast against reflect dispatch, superglobal registration, and the four host handler shapes                                                                                                   |
| `tests/flatstack/*_bench_test.go`                            | Precompiled programs, and the interpreter against bytecode on one source                                                                                                                                                     |
| `cmd/phpscript/server/precompile_test.go`                    | A served entrypoint, lazy against precompiled, on both engines                                                                                                                                                               |

Three subtest naming conventions, each meaning something:

- `engine=runner` and `engine=flatstack`, written for `benchstat -col /engine`.
- `go`, `pooled`, `pooled_reset`, `fresh` on `BenchmarkLookupHandler`: four host shapes for reaching PHP from Go, from a native handler to a runtime per request.
- `interpreter` or `flatstack` crossed with `lazy` or `precompiled` on `BenchmarkServeEntrypoint`.

How to read benchstat: `~` means the change is within the noise for the samples given, and the p-value beside it says how confident that is. A `~` is an instruction to take more samples, or to accept that the change did not move this number.

An engine benchmark means nothing unless its program is gated on `flatstack.Supports`. A program that falls back records the interpreter's result and its cost under the flatstack name. `tests/flatstack/benchmark_test.go` fails the benchmark where a fallback would otherwise be reported, and that is the pattern a new one copies. [../flatstack.md](../flatstack.md) argues why.

A pinned number for one of the three parallel benchmarks describes a single goroutine. It is not reported.

## What this cannot answer

| Question                              | Document                                                                                                                        |
|---------------------------------------|---------------------------------------------------------------------------------------------------------------------------------|
| A latency distribution                | [collect-phpscript-test.md](collect-phpscript-test.md) for fixtures, or `wrk` through [performance.md](performance.md) for HTTP |
| Where inside a function the time goes | [collect-phpscript-run.md](collect-phpscript-run.md)                                                                            |
| Whether the tree still passes         | [collect-go-tests.md](collect-go-tests.md)                                                                                      |
