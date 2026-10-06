# Profile sprint

The cpu and memory map of the tree, per package, per function and per fixture, before and after, with p50 and p99.

Read this when the question is where the cost is. It runs under the contract in [README.md](README.md); that document is not restated here.

## What this produces

Three layers, measured in three different units, as one artifact set:

- Per package: wall-clock duration, from the test run.
- Per function: cpu samples and heap objects, from pprof.
- Per fixture: p50, p95, p99, allocs/op and B/op, from the fixture runner.

This sprint changes no code. That is what separates it from [performance.md](performance.md), which changes code and must prove a delta on a target. The map produced here is what that sprint reads first.

## Layer 1: per package

`gotestsum` writes the durations and `gotestsum tool slowest` ranks them. The commands are in [collect-go-tests.md](collect-go-tests.md).

The shape to expect: 53 packages, 43 of them with tests. `tests/runner` is the largest single package and `tests` is next, because `tests` runs the whole fixture corpus twice, once per engine. Those are proportions, not durations - the contract keeps measurements out of these documents.

`tests/runner`'s share is real wall clock, not work. It holds the execution-limit and client-abort tests, which drive `set_time_limit`, `usleep` and `connection_aborted` against the clock. Those seconds are sleeps. They are excluded from any reading of what is slow, stated explicitly in the report, because the alternative is a sprint that tries to optimise a sleep.

A duration here is one sample and carries no interval. It ranks packages; it does not compare two commits.

## Layer 2: per function

`-cpuprofile` and `-memprofile` per package, then `go tool pprof -top -nodecount=40`.

Before and after is `-diff_base`, not two `-top` listings read side by side:

```sh
go tool pprof -top -nodecount=40 -diff_base=cpu-before.pprof cpu-after.pprof
```

One subtlety decides whether a per-function reading means anything. A profile of `phpscript test` contains the harness: fixture discovery, table rendering, the `php` subprocess, the JSON writer. Those frames sit on top of the interpreter and crowd out what is being looked for. A per-function reading uses `phpscript run` over one file instead, which is what [collect-phpscript-run.md](collect-phpscript-run.md) is for.

The same applies in the other direction: `phpscript run` over one file includes process start, the parse, and the whole of `stdlib.Register`, which a benchmark loop amortises away. Neither is wrong; they answer different questions, and a profile says which one it is.

## Layer 3: per fixture

`--count N --time 100ms --json` is the only source of percentiles in this project. The commands and the field list are in [collect-phpscript-test.md](collect-phpscript-test.md).

Both Go runners, through `--matrix --skip-php`. `php` is excluded on purpose: it is another process with another engine, and its latency is not this project's to optimise. It stays in the correctness matrix and out of the timing one.

`--cache` decides what is being timed. `worker` amortises the parse across the fixtures a worker runs, which is the production shape. `off` re-parses per fixture, which measures the parser. Picking the wrong one is the common error, and the report names which was used.

It also decides what the run costs in resident memory, so layer 3 runs one process per area under `GOMEMLIMIT`. The contract's memory section has the reasoning and the loop.

## The two memory numbers

They measure different things and the point is to put them side by side.

| Number                                     | What it is                                                                                                                        |
|--------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| `runtime.MemStats.TotalAlloc` delta        | Everything the Go process allocated while the work ran, freed or not. Includes allocator overhead and every interpreter structure |
| `rt.MemoryPeak()` over `runner.MemoryWalk` | The high-water mark of live PHP values only. No allocator overhead, no interpreter structures                                     |

Neither is ever labelled "memory" alone. A report that gives one number without saying which it is cannot be acted on: the first says what the collector will have to do, the second says what the script thinks it is holding, and a large ratio between them is itself the finding.

## Combinations that are refused

The command errors rather than producing something misleading, so these are separate runs by construction.

| Combination                                    | Why                                                                                          |
|------------------------------------------------|----------------------------------------------------------------------------------------------|
| `--cover` with `--count` or `--time`           | A coverage count is how many times a line ran, not how many times a loop replayed it         |
| `--profile` with `-p`                          | Go's allocation counters are process-wide and cannot be attributed to one concurrent fixture |
| `--cover=func` or `--cover=file` with `--json` | Two report formats, one stdout                                                               |

## Before and after

Two commits in the same worktree, both rebuilt from scratch, under the lock. The contract owns the rest: the drift guard, the dropped GC samples, and the manifest that catches a stale binary before a number is published.

Nothing produced here is committed. `atkins.yml` already records the reasoning on `test:phpscript:matrix`: the fixture report is generated deliberately without `--profile`, because a timing that differs by a millisecond per run would be a diff in every commit.

## Reporting

The run produces thousands of rows. The report carries three tables and nothing else:

- Per package: the five slowest, with `tests/runner`'s sleeps called out and excluded.
- Per function: the twenty heaviest by cpu and the twenty heaviest by allocated objects, from `-diff_base` where there are two commits to compare.
- Per fixture: the five slowest by p99, and every row whose p99 moved beyond the drift bound.

Canonical shapes from the contract, environment stated once above the first table. The raw artifact is named so a reader can re-derive any row; the report does not reproduce it.
