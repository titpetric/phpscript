# `phpscript test <path>...`

Discover and run `.phpt` fixtures. With no path, the command searches the
current directory. Results are printed as they complete, using a colored table
in a terminal and Markdown when output is redirected.

It also accepts the [global flags](README.md#global-flags): `-f`, `-w`,
`--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`.
The flags below are this command's own.

A directory path is not recursive on its own: `./...` walks a tree, and a path
that matches no fixture is an error rather than a silent pass.

```bash
phpscript test tests/fixtures/...
phpscript test tests/fixtures/arrays
phpscript test tests/fixtures/arrays/array_indexing.phpt
```

The run answers with one row per folder resolved from the arguments. A tree of
a few hundred fixtures is a few hundred passing rows, and reading them is not
what a run is for; what a reader needs is which folder the failures are in, and
the failures themselves are printed below the table either way.

```text
| Path                      | Fixtures | Passed | Failed |
| ------------------------- | -------- | ------ | ------ |
| tests/fixtures/arithmetic | 21       | 21     | 0      |
| tests/fixtures/arrays     | 19       | 19     | 0      |
```

Add `--verbose` (`-v`) for the fixture tables: one table per folder, with the
folder name as the header of the fixture column, each followed by that folder's
subtotal. `--output` (`-o`) writes those tables whether or not `-v` is set, so a
checked-in report is the same file either way.

Use `--count N` (`-c`) to run each fixture N times in one aggregate row, or
`--time D` (`-t`) to repeatedly run each fixture for at least duration D.
When both flags are provided, they select benchmark sampling: each fixture
prints N rows, and each row is an independent sample that runs for at least D.
The `Count` column is the number of fixture executions completed during that
sample. With `--time`, `P50`/`P95`/`P99` report per-operation latency in
microseconds so the wall-clock sample window is not mistaken for a single-run
duration. `GC Runs` reports completed Go garbage-collection cycles as
`N (M%)`, where M is their share of the fixture execution count for that row,
shown to two decimal places.

```bash
phpscript test -c 5 tests/fixtures/...
phpscript test -t 1s tests/fixtures/...
phpscript test --count 5 --time 1s tests/fixtures/...
```

Use `--parallel N` (`-p`) to run up to N fixtures in each area concurrently.
Output remains in discovery order. Fixtures that share external state can set
`serial: true` in their metadata to run as a barrier between parallel batches.
`--profile` cannot be combined with parallel execution because Go's allocation
counters are process-wide and cannot be attributed to one concurrent fixture.

Use `--profile` to add per-operation allocation and byte counts. `--json`
writes a machine-readable report to stdout (no table).

Use `--cache` to say how far a parsed include and a compiled expression
travel. `worker`, the default, gives each worker loop one set of caches and
one runtime, reused by the fixtures that worker runs serially, so what a run
holds scales with `--parallel` rather than with the number of fixtures. `off`
gives every fixture run its own and drops them — and its runtime — when the run
ends: a clean state, at the cost of re-parsing what the caches would have kept.
There is no `shared` mode, because a worker loop already is one: without
`--parallel` there is a single worker.

```bash
phpscript test --cache=off tests/fixtures/...
```

Use `--cover` to measure statement coverage on the phpscript runtime, in the
format `go test -coverprofile` writes (`mode: count`; each entry counts how
many times its line range ran) with the PHP files as the entries. The profile
lists every file `get_included_files` reports, so an included file's unexecuted
statements appear at count zero. `--coverfile FILE` names the profile (implies
`--cover`; the default is `phpscript.cov`), and `--split` also writes each
fixture's own coverage next to it as `<fixture>.cov`. Coverage is a property of
the interpreter alone: under `--matrix` only the runtime column collects, since
flatstack is a performance-oriented backend without coverage support and the
`php` column is another process. The benchmark flags `--count` and `--time`
cannot be combined with coverage, because a count is how many times the fixture
reached a line, not how many times a benchmark loop replayed it.

With `--cover`, the folder summary carries the two counts that answer different
questions. `Files` is how many of the PHP files the folder's fixtures loaded
were reached at all, which says what the suite has not looked at; `Lines` is how
many of the statements in them ran, which says how thoroughly it looked at the
rest. A folder scoring 8% of files and 90% of lines is tested in one corner, and
neither number alone would say so. Both columns read `N/M (J%)`. A folder whose
fixtures loaded no PHP file of their own has nothing to measure and reports `-`.

```text
| Path                    | Fixtures | Passed | Failed | Files      | Lines      |
| ----------------------- | -------- | ------ | ------ | ---------- | ---------- |
| tests/fixtures/includes | 3        | 3      | 0      | 2/4 (50%)  | 3/10 (30%) |
| tests/fixtures/oop      | 24       | 24     | 0      | 2/2 (100%) | 9/9 (100%) |
```

Files are charged to the folder whose fixtures loaded them, because a fixture's
own directory is its include root: two folders including the same relative path
are including their own copy of it.

With `-v`, each fixture table gains a `Coverage` column — the coverage of the
PHP that fixture loaded, not of the `.phpt` itself — and every folder that
loaded a file gets a per-file section below the tables, which is where an
unvisited file is named rather than counted.

```text
## coverage: tests/fixtures/includes

tests/fixtures/includes/counter.php           2/2 lines covered 100.0%
tests/fixtures/includes/modules/functions.php 0/6 lines covered   0.0%
```

`--cover` takes a mode. Bare `--cover` means `--cover=line`: write the profile,
and under `-v` print the one-line percentage below the tables. That percentage
measures the written profile, which counts the `.phpt` entrypoints, and the
tables do not; printing both without `-v` would invite a comparison between two
numbers answering different questions. `--cover=func` and
`--cover=file` still write the profile, but own stdout with a coverage report
in the format `go tool cover -func` prints — one row per declared function
(methods as `Class::method`, a file's top-level code as `{main}`) or one row
per file, each with its statement-weighted percentage, ending in a `total:`
row. The report covers the application sources; the `.phpt` fixtures
themselves are excluded, being the tests. A file or function with no runnable
statement — an interface, a class of pure declarations — reports the adjusted
0/0 as 100%: nothing is left uncovered, and the row contributes no statements
to the total. The fixture tables are suppressed so
the report pipes clean (failures go to stderr, `-o` still writes the Markdown
tables), which also rules out combining a report mode with `--json`.

```bash
phpscript test --cover tests/fixtures/...
phpscript test --coverfile phpscript.cov --split tests/fixtures/...
phpscript test --cover=func --include vendor/autoload.php
```

Use `--output FILE` (`-o`) to write the same tables to a file as Markdown while
the terminal output continues as normal. The file ends with a summary table of
per-folder totals. `--matrix`, `--profile`, `--count` and `--time` all
contribute their columns to it.

```bash
phpscript test --matrix -o docs/test-fixtures.md tests/fixtures/...
```

Use `--matrix` to run every fixture through all three runtimes (the flat
bytecode engine, the default interpreter, and the `php` binary), reporting one
row per fixture with a cell per runtime. Every other `test` flag is honored,
and the benchmarking flags add their columns after the runtime columns; those
numbers are the default runtime's, because a row has one cost column and three
runtimes. Per-runtime cost stays available in `--json`. A
fixture that opts out of a runtime, and a runtime that is not installed, are
reported as `SKIP`; anything else that is not a pass fails the run, and the
command exits non-zero.

```bash
phpscript test --matrix tests/fixtures/...
phpscript test --matrix -v tests/fixtures/...
```

| arrays              | Flat stack | Runtime | PHP  |
|---------------------|------------|---------|------|
| array_indexing.phpt | PASS       | PASS    | PASS |
| sort.phpt           | PASS       | PASS    | PASS |

| bindings          | Flat stack | Runtime | PHP  |
|-------------------|------------|---------|------|
| storage_list.phpt | PASS       | PASS    | SKIP |

The matrix follows the same rule as a plain run: without `-v` it prints the
folder summary, and `-v` restores the tables above and prints the failure of
each runtime in continuation rows below its fixture. A continuation row leaves
the fixture column empty, so it reads as part of the row above it.
