# Performance sprint: allocation reduction

**2026-08-16.** Total allocations across the `.phpt` fixture suite fell by
**80.6%** (223,063 → 43,356 allocs/op), bytes allocated by **77.4%**, and GC
runs from 10 to 2. No fixture regressed; the smallest improvements were 56% on
allocation count and 27% on bytes.

Interpreter throughput rose **4.9x** on the heavy fixtures; the `phpscript test` CLI over the whole suite is **1.44x** faster end to end, because fixed
startup costs now dominate a run. See [Throughput](#throughput).

## The process

The work was driven by [allocation-performance.md](../allocation-performance.md),
which had a TODO-annotated audit of every binding plus a short list of
larger structural items. It ran in three rounds, each one measure → change →
rebuild → re-measure:

```sh
phpscript test --profile ./tests/...   # per-fixture allocs/op, B/op, GC runs
go install .                           # rebuild the binary being measured
```

**Round 1: work the document's TODO list.** Four parallel efforts, each
scoped to one package so they could not conflict: `runner` (the `baseEnv`
rebuild), `model` (list-mode `Array`), `parser` (`TokenGetAll`'s nested array
shape), `stdlib` (the binding audit's TODO rows). Result: −18.6%.

**Rounds 2 and 3: follow the profiler instead.** With the document's list
exhausted, the remaining targets came from
`go test ./tests/ -bench ... -memprofile` over the six heaviest fixtures and
`go tool pprof -sample_index=alloc_objects`. This is where the largest single
win came from, and it was not in the document at all. Result: −62% and −16%
on top of what was already there.

The audit was a good starting point and a poor finishing point. It correctly identified that `runner.baseEnv` dominated, but
it attributed ~78% of allocations to the *runtime* environment when a
comparable cost sat in the *compile-time* type environment, unmentioned. Two
of its TODOs were also worth less than advertised by the time they were
reached; see "Changes not made" below.

## Where the wins came from

| Change                                                                                                                                                                                        | Effect                                                                                                                                                                                                                                                     |
|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `runner`: pooled evaluation environments (`acquireEnv`/`releaseEnv`) reached through a `scopeRef` indirection, plus on-demand installation of only the functions an expression actually calls | `baseEnv` rebuilt the whole env per `Eval`, one closure per registered function. `BenchmarkScriptEnvFullStdlib` and `...Minimal` are now identical at 24 allocs/op (were 649 and 145): a script pays for the functions it calls, not the size of the table |
| `runner`: cache expr's compile config per function-table generation; derive the type-env nature once instead of per key                                                                       | `expr.Compile(src, expr.Env(typeEnv), ...)` made expr walk a ~95-entry map via `MapKeys`/`MapIndex`/`copyVal` on **every compile**, 64% of all allocations                                                                                                 |
| `parser`: operator tokens from a package-level substring table, presized token slice, chunked AST node allocation                                                                             | Parsing a 10.8 KB file: 3197 → 564 allocs (−82%)                                                                                                                                                                                                           |
| `parser`: `TokenGetAll` returns `[]any` of `[]any` instead of `*model.Array` of `*model.Array`, with pre-boxed id/line tables and chunked triples                                             | 6003 → 1828 allocs on a 5.6 KB template                                                                                                                                                                                                                    |
| `model`: `Array` list mode: while every key is the dense sequence `0..n-1`, values live in a `[]any` and neither the `map[any]any` nor the key slice is allocated                             | 5-element build 11 → 9 allocs, 50-element 66 → 57, `Range` ~17x faster at zero allocations                                                                                                                                                                 |
| `stdlib`: hoisted `htmlspecialchars`' `strings.Replacer` to package scope; `crc32` via a package-level table; `Exception` returns a pointer; `toString`/`toInt` off `fmt`                     | 11 → 2 allocs, 1 → 0 allocs, and `$e->message = "x"` now works                                                                                                                                                                                             |

## Fixture data

`phpscript test --profile ./tests/...`, Intel N150, Go 1.27. Bytes are KiB.
GC columns are collections observed during the run.

| Fixture                                 | Allocs before | after     | Δ          | KiB before  | after      | Δ          | GC before | after |
|-----------------------------------------|--------------:|----------:|-----------:|------------:|-----------:|-----------:|----------:|------:|
| `include_minitpl`                       |         30427 |      4208 |       -86% |      3093.1 |      310.7 |       -90% |         2 |     0 |
| `operators_and_mutation`                |         20791 |      2039 |       -90% |      1880.3 |      200.0 |       -89% |         1 |     0 |
| `ternary_conditions`                    |         20373 |      2973 |       -85% |      1993.9 |      289.7 |       -85% |         1 |     0 |
| `array_indexing`                        |         13895 |      3172 |       -77% |      1356.3 |      294.5 |       -78% |         0 |     0 |
| `object_nesting`                        |         11612 |      1775 |       -85% |      1146.2 |      144.7 |       -87% |         0 |     0 |
| `runtime_introspection`                 |         10631 |      2011 |       -81% |      1122.3 |      166.1 |       -85% |         0 |     0 |
| `session_manager`                       |          9109 |      1778 |       -80% |       944.3 |      156.7 |       -83% |         1 |     0 |
| `request_and_response_handling`         |          8196 |      1351 |       -84% |       902.7 |      135.3 |       -85% |         1 |     0 |
| `compact`                               |          7911 |      1270 |       -84% |       852.0 |      118.3 |       -86% |         0 |     0 |
| `autoloading`                           |          7620 |      1203 |       -84% |       843.7 |      127.7 |       -85% |         0 |     0 |
| `get_included_files`                    |          7055 |      1318 |       -81% |       946.3 |      434.2 |       -54% |         0 |     1 |
| `php_array_splice`                      |          6995 |      2007 |       -71% |       654.6 |      233.0 |       -64% |         0 |     0 |
| `database_test`                         |          6428 |      1563 |       -76% |       617.9 |      135.7 |       -78% |         0 |     0 |
| `storage_lifecycle`                     |          6347 |      1354 |       -79% |       675.7 |      154.5 |       -77% |         1 |     0 |
| `storage_list`                          |          5696 |      1265 |       -78% |       534.3 |      147.0 |       -72% |         0 |     0 |
| `exception_response_code`               |          4817 |      1187 |       -75% |       498.9 |      138.6 |       -72% |         0 |     0 |
| `database_migrate`                      |          4235 |      1368 |       -68% |       509.9 |      135.9 |       -73% |         1 |     0 |
| `condition_syntax`                      |          4113 |       781 |       -81% |       470.5 |      119.4 |       -75% |         0 |     0 |
| `php_foreach_syntax`                    |          4042 |       940 |       -77% |       399.1 |      131.6 |       -67% |         0 |     0 |
| `flatstack_destructuring_assignment`    |          4041 |       774 |       -81% |       446.4 |      132.9 |       -70% |         1 |     0 |
| `flatstack_runner_compatible_fast_path` |          3523 |      1023 |       -71% |       379.2 |      128.5 |       -66% |         0 |     0 |
| `autoloading_default`                   |          3396 |       877 |       -74% |       385.6 |      154.5 |       -60% |         1 |     0 |
| `platform_database`                     |          2857 |       934 |       -67% |       153.3 |       69.0 |       -55% |         0 |     0 |
| `user_function_declaration_and_call`    |          2048 |       596 |       -71% |       238.2 |      104.3 |       -56% |         0 |     0 |
| `include-return`                        |          1785 |       328 |       -82% |       199.3 |       65.3 |       -67% |         0 |     0 |
| `defer-usage`                           |          1716 |       372 |       -78% |       196.8 |       66.2 |       -66% |         0 |     0 |
| `storage_constructor_error_caught`      |          1640 |       517 |       -68% |       185.0 |       96.5 |       -48% |         0 |     1 |
| `exception`                             |          1602 |       564 |       -65% |       177.8 |       88.9 |       -50% |         0 |     0 |
| `die_exit`                              |          1514 |       492 |       -68% |       176.3 |       90.2 |       -49% |         0 |     0 |
| `autoloading_missing`                   |          1496 |       475 |       -68% |       183.7 |       86.8 |       -53% |         0 |     0 |
| `stdin`                                 |          1459 |       457 |       -69% |       175.0 |       96.5 |       -45% |         0 |     0 |
| `storage_context`                       |          1274 |       468 |       -63% |       131.6 |       90.5 |       -31% |         0 |     0 |
| `storage_method_error`                  |          1254 |       548 |       -56% |       133.1 |       94.2 |       -29% |         0 |     0 |
| `storage_method_error_caught`           |          1141 |       495 |       -57% |        94.9 |       69.0 |       -27% |         0 |     0 |
| `storage_constructor_error`             |          1034 |       455 |       -56% |       120.4 |       87.1 |       -28% |         0 |     0 |
| `flatstack_arithmetic`                  |           990 |       418 |       -58% |       118.2 |       82.7 |       -30% |         0 |     0 |
| `token_get_all`                         |           n/a |      2930 |        n/a |         n/a |      186.7 |        n/a |       n/a |     0 |
| **Total (36 common)**                   |    **223063** | **43356** | **-80.6%** | **22936.8** | **5176.6** | **-77.4%** |    **10** | **2** |

`token_get_all.phpt` is new, added during the sprint to pin the PHP-visible
shape of `token_get_all` after it stopped returning `*model.Array`. It is
excluded from the total so the comparison is like-for-like.

## Throughput

The harness's own duration column is whole milliseconds and most fixtures now
finish under 1 ms, so it is no longer a usable measure. The numbers below come
from building the pre-sprint commit (`eba815c`) and running both binaries
against the same 36 fixtures.

Fixture execution excluding process startup, `ResetCaches()` per iteration so
every run pays a cold parse and compile, the one-shot CLI case:

| Fixture                  | before   | after     | speedup  |
|--------------------------|---------:|----------:|---------:|
| `include_minitpl`        |    159/s |     671/s |     4.2x |
| `ternary_conditions`     |    276/s |    1334/s |     4.8x |
| `operators_and_mutation` |    316/s |    2319/s |     7.3x |
| `array_indexing`         |    328/s |    1656/s |     5.1x |
| `object_nesting`         |    422/s |    2384/s |     5.6x |
| `runtime_introspection`  |    542/s |    2259/s |     4.2x |
| **combined**             | **49/s** | **242/s** | **4.9x** |

End to end, `phpscript test ./tests/...` over the whole suite (including
process startup, driver connections and fixture I/O) went **91 ms → 63 ms,
1.44x** on five alternating runs, 89-93 ms versus 61-64 ms.

The gap between 4.9x and 1.44x is the useful part: the interpreter is roughly
five times faster, but a CLI invocation now spends most of its 63 ms on fixed
costs this sprint did not touch. Those are what to attack next if the CLI's
wall-clock is the target.

## Changes not made, and why

- **`array_merge` returning `[]any` for all-list inputs.** This was a TODO in
  the audit, implemented, measured, and reverted. Its premise, that
  `*model.Array` cost ~5 allocations more, stopped being true when list mode
  landed in the same round; measured 8 allocs either way. The slice cannot
  serve `$x = array_merge($a, $b); $x[] = "z"`, so the appendability was worth
  more than 40 bytes.
- **Dropping `expr.Env` entirely.** Tempting, and much faster still, but
  wrong: `expr/parser.parseCall` consults its own `predicates` table before
  the disabled-builtins list, and only `conf.Config.IsOverridden`, which
  reads `Config.Env`, stops a name being parsed as expr predicate syntax.
  PHP's `count`, `map`, `filter`, `find`, `sum`, `reduce` and `sortBy` all
  collide. `expr.DisableAllBuiltins()` does not cover it. Caching the derived
  config was taken instead, and `runner/compile_test.go::TestCompileMatchesExprEnv`
  now guards it by diffing `vm.Program.Disassemble()` against a stock
  `expr.Compile(expr.Env(...))` over a corpus.
- **Sharing one closure set across a `Runtime`'s pooled environments via a
  scope stack.** Cannot be made safe for concurrent use of one `Runtime` from
  multiple goroutines without per-goroutine state. Lazy installation gets most
  of the win without touching the scope model.
- **Caching the expr config across `Runtime`s.** `conf.Config` is mutated
  during compile (`NtCache` writes unsynchronised maps); a package-level cache
  would need a global lock around every compile, trading a per-`Runtime`
  one-time cost for cross-`Runtime` contention.

One assumption did not hold up. The audit called `token_get_all`
"the worst remaining shape" partly because "the minitpl compiler tokenises
every template". Instrumenting the tokenizer showed `include_minitpl.phpt`
never calls it; the fixture's template only uses `{name}`, which takes a
different path. The change is still worth having (it is 3.3x on the tokenizer
and `phpscript fmt`/`list`/`ast` use it), but it is not what made that fixture
heavy.

## What is left

The two largest remaining blocks are both out of reach of this kind of work:

- **expr's own parse and compile pipeline**, ~48% cumulative. This is a
  *cold-start* cost (`ExprCache` means a long-lived runtime pays it once per
  distinct expression), so it dominates one-shot CLI runs and not servers.
  Reducing it means emitting fewer or shorter expressions, not micro-optimising
  a dependency.
- **`reflect.Value.Call`**, ~43% cumulative. This is the reflection boundary
  the project deliberately trades throughput for; see "Where the guidance
  stops" in [allocation-performance.md](../allocation-performance.md).

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l` clean. `go test ./... -count=1`
and `go test ./... -race -count=1` pass. All 37 fixtures pass. Tests were added
for every semantic change: `model/array_test.go` (differential test against a
replica of the old map-backed implementation), `runner/compile_test.go`
(bytecode equivalence), `runner/transpile_test.go`, `parser/lexer_test.go`,
`parser/parser_test.go`, `stdlib/stdlib_internal_test.go`,
`stdlib/stdlib_script_test.go`, and `tests/fixtures/token_get_all.phpt`.
