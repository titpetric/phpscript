# Collect: phpscript test

Coverage profiles, or benchmark samples with p50, p95 and p99, from the fixture runner.

Read this when the artifact needed comes from the fixture suite. It runs under the contract in [README.md](README.md); that document is not restated here.

## What this collects

Two modes, and the command refuses to mix them:

- Coverage: which PHP statements the fixtures executed.
- Benchmark sampling: per-fixture latency percentiles, allocations and bytes.

This is the only source of percentiles in the project.

## Preconditions

The contract's lock and private binary. The private-binary rule bites hardest here: `phpscript test` runs the binary on `PATH`, not the tree, so a sprint that skips the `command -v` check measures whatever was installed last and will not notice.

`./...` is load-bearing. `phpscript test ./tests/...` walks the tree; `phpscript test ./tests` matches only fixtures sitting directly in that directory and passes over nothing.

The database fixtures need `.env.testing`:

```sh
set -a; export $(grep -v ^# .env.testing | xargs -d "\n"); set +a
```

## The run

Coverage, writing the profile and printing the per-file report:

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  CGO_ENABLED=0 go build -o bin/phpscript .
  PATH="$PWD/bin:$PATH"
  command -v phpscript | grep -q "^$PWD/bin/" || exit 1
  set -a; export $(grep -v ^# .env.testing | xargs -d "\n"); set +a
  mkdir -p cover
  phpscript test --cover=file --coverfile cover/fixtures.cov ./tests/...
'
```

`--cover` takes `line`, `func` or `file`. `line` writes the profile only; `func` and `file` also print a report in the format `go tool cover -func` prints. `--coverfile` implies `--cover`, defaults to `phpscript.cov`, expands `{time}` to a UTC timestamp, and creates the directory. `--split` additionally writes each fixture's own profile beside it.

Benchmark sampling, which is the percentile mode. One process per area, under `GOMEMLIMIT`, for the reason the contract's memory section gives: a runtime is retained as long as its parse cache, so one process over the whole suite holds every runtime it built and the kernel ends the run.

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  CGO_ENABLED=0 go build -o bin/phpscript .
  PATH="$PWD/bin:$PATH"
  command -v phpscript | grep -q "^$PWD/bin/" || exit 1
  set -a; export $(grep -v ^# .env.testing | xargs -d "\n"); set +a
  for area in tests/fixtures/*/; do
    GOMEMLIMIT=2GiB phpscript test --matrix --skip-php --profile --json \
      --cache=off --count 6 --time 100ms "$area..." \
      > "bench-fixtures-after-$(basename "$area").json"
  done
'
```

`--count N` with `--time D` selects sampling: each fixture produces N rows, each an independent sample that ran for at least D. `--count` without `--time` aggregates N runs into one row instead.

Cross-cutting flags:

| Flag             | Effect on the artifact                                                        |
|------------------|-------------------------------------------------------------------------------|
| `--matrix`       | One row per runtime instead of one per fixture                                |
| `--skip-php`     | Leaves the `php` binary out, so the timing covers the two Go engines only     |
| `--cache=worker` | One parse cache per worker loop. The default, and the production shape        |
| `--cache=off`    | A new cache per fixture run, so every run re-parses. This measures the parser |
| `-o FILE`        | Writes the markdown report while the table still goes to the terminal         |
| `-v`             | Adds each runtime's failure below its fixture                                 |

Three combinations are refused, with these messages:

```
profile cannot be combined with parallel fixture execution
cover cannot be combined with --count or --time
cover=func cannot be combined with --json
```

The first because Go's allocation counters are process-wide and cannot be attributed to one concurrent fixture. The second because a coverage count is how many times a line ran, not how many times a loop replayed it. The third because both want stdout.

## The artifact

`bench-fixtures-<side>.json`, already ignored by `bench*.json`. One object per fixture per runner per sample, under `results`, with `total`, `passed` and `failed` alongside.

Which fields appear depends on which flags were given:

| Fields                                                               | Present                     |
|----------------------------------------------------------------------|-----------------------------|
| `name`, `path`, `runner`, `passed`, `runs`, `duration_ns`, `gc_runs` | Always                      |
| `p50_ns`, `p95_ns`, `p99_ns`                                         | With `--count` and `--time` |
| `allocs_per_op`, `bytes_per_op`                                      | With `--profile`            |

`cover/<label>.cov`, already ignored by `/cover/*` and `*.cov`.

`bench-manifest-<side>.txt` alongside, as the contract requires.

## Reading it

`--json` is nanoseconds. The printed table is microseconds, which is what the contract's latency shape reports, to one decimal. Converting one into the other by hand is where a factor of a thousand gets published.

`duration_ns` is the sample window, not a per-run duration. It is roughly whatever `--time` asked for. The per-run numbers are the percentiles, so `--time` reports them at all.

`gc_runs` prints as `N (M%)` in the table, where M is the collector's share of that row's fixture execution count. A row whose GC share differs between before and after is not comparable and is re-measured, per the contract.

`--cache` decides what was measured, what it costs in resident memory, and whether the set drifts. `off` re-parses every run and prices the parser, and drops the runtime with the fixture. `worker` amortises the parse and prices execution, and keeps every runtime its worker built - so a late sample window carries the collector load of every fixture before it and the run drifts upward. A before/after pair that disagrees on `--cache` compares two different things; a percentile run uses `off`, for the reason the contract's stress-run section gives.

Coverage is the interpreter's alone. The bytecode engine does not collect, and `php` is another process. A coverage number from a `--matrix` run describes one runtime.

In the coverage summary, files and lines answer different questions: a high line percentage over few files says the covered files are covered well, not that the tree is. Both go in a report.

## What this cannot answer

| Question                                 | Document                                             |
|------------------------------------------|------------------------------------------------------|
| Per-function Go cost                     | [collect-phpscript-run.md](collect-phpscript-run.md) |
| Go allocations attributable to a package | [collect-go-benchmarks.md](collect-go-benchmarks.md) |
| HTTP latency                             | `wrk`, through [performance.md](performance.md)      |
| Whether the tree still passes            | [collect-go-tests.md](collect-go-tests.md)           |
