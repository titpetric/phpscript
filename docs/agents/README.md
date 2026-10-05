# Agent sprints

The sprint prompts, and the one contract all of them run under.

Read this before running any document in this directory, and before quoting a measurement anywhere: in a pull request, in an issue, or in a document. The protocol below is what makes two numbers comparable, and a number taken outside it is not evidence.

## The documents

Each one is read and executed on its own. None of them repeats what is here.

| Document                                               | Read when                                                 | What it leaves behind                                                          |
|--------------------------------------------------------|-----------------------------------------------------------|--------------------------------------------------------------------------------|
| [qa-sprint.md](qa-sprint.md)                           | Hunting bugs, dead code or duplication                    | One pull request per finding, each with the fixture that pins it               |
| [performance.md](performance.md)                       | Cutting the per-request cost of `testdata/testserver.php` | A harness, a published baseline, and one pull request per measured improvement |
| [profile.md](profile.md)                               | Answering where the cost is, before changing anything     | The cpu and memory map of the tree, per package, per function and per fixture  |
| [collect-go-tests.md](collect-go-tests.md)             | Collecting pass/fail and duration from `go test`          | `bench-gotest-<side>.json`                                                     |
| [collect-go-benchmarks.md](collect-go-benchmarks.md)   | Collecting `ns/op`, `B/op` and `allocs/op`                | `bench-go-<side>.txt` and its `benchstat` comparison                           |
| [collect-phpscript-test.md](collect-phpscript-test.md) | Collecting fixture coverage, or percentiles               | `cover/<label>.cov`, or `bench-fixtures-<side>.json`                           |
| [collect-phpscript-run.md](collect-phpscript-run.md)   | Profiling one script with no harness in the profile       | `cpu-<label>.pprof`, `mem-<label>.pprof`                                       |

## What a sprint is

A sprint is a worktree, a measurement and one pull request per finding. An agent reads one document and runs it to a pull request without further instruction.

Each sprint runs in its own git worktree. That is what makes `$PWD/bin/phpscript` private, and it is what keeps two sprints off one branch.

One finding is one pull request. A sprint that produces four findings opens four, each reviewable on its own. A branch holding an unrelated second change is the thing a reviewer cannot approve.

Out of scope in every sprint: anything in the "Non-negotiable" list in [../../AGENTS.md](../../AGENTS.md), and anything in the "Won't implement" table in [../design.md](../design.md). A finding that needs one of those is an issue from the feature-gap template, not a half fix.

## The measurement protocol

### The lock

Build and measure are one unit, not two. An exclusive lock wraps both:

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '...'
```

The box is shared with other agents. A concurrent `go install` overwrites the binary mid-run, and a concurrent `go test ./...` takes the cores a sample is being timed on. `-w 3600` so a stuck holder surfaces as a failure rather than as a hang.

Anything that compiles the tree or runs the test tree takes the lock, including a correctness `go test ./...`: a full suite run wrecks somebody else's sample. Reading code, grepping and editing do not.

### The private binary

Never `go install .`. It writes to a shared `GOBIN`, which on a box running several sprints is the binary every other agent is measuring against.

```sh
CGO_ENABLED=0 go build -o bin/phpscript .
PATH="$PWD/bin:$PATH"
command -v phpscript | grep -q "^$PWD/bin/" || exit 1
```

`bin/` is already gitignored. The third line is not optional and is the first line of every measurement log, because `phpscript test` runs the binary on `PATH` and not the tree: without it a sprint silently measures whatever was installed last.

The whole thing as one locked session:

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  CGO_ENABLED=0 go build -o bin/phpscript .
  PATH="$PWD/bin:$PATH"
  command -v phpscript | grep -q "^$PWD/bin/" || exit 1
  # measure here
'
```

### CGO and the toolchain

`CGO_ENABLED=0` always, for every build and every `go test`, which is what `.atkins/skills/go.yml` builds with. The only exception is `-race`, which needs cgo; a race run produces no number and is never a baseline.

A before/after pair is two binaries built the same way by the same toolchain. A profile taken against a cgo binary and one taken without are two measurements of two programs, so a binary of unknown provenance is not a baseline.

`GOFLAGS=""`, for the reason `atkins.yml` sets it: a `-mod=mod` inherited from a go.work environment breaks every go command.

### Pinning

Two benchmark jobs, not one.

`taskset -c 3` narrows the affinity mask to one CPU. Go reads that mask for `GOMAXPROCS`, so a `b.RunParallel` benchmark runs a single goroutine and a contention fix measures as zero. Three benchmarks are parallel and run unpinned:

| Benchmark                              | File                                | Subtests                                 |
|----------------------------------------|-------------------------------------|------------------------------------------|
| `BenchmarkLookup`                      | `tests/runner/lookup_bench_test.go` | bind, invoke, callable, runtime, request |
| `BenchmarkLookupHandler`               | `tests/runner/lookup_bench_test.go` | go, pooled, pooled_reset, fresh          |
| `BenchmarkFlatstackParallelHostBridge` | `tests/flatstack/benchmark_test.go` | none                                     |

`BenchmarkLookupTreeSize` is serial and stays in the pinned job, which is why the selector anchors the first name:

```sh
PARALLEL='BenchmarkLookup$|BenchmarkLookupHandler|BenchmarkFlatstackParallelHostBridge'
```

The unpinned job leaves `GOMAXPROCS` alone, records it in the manifest, and takes `-count 10` where the pinned job takes 6. A parallel benchmark on a shared box is the noisiest number in the set, and benchstat's interval decides whether it moved, not the delta.

### Samples and statistics

`-count 6` minimum, so `benchstat` has enough samples to report an interval. `-count 10` for the three parallel benchmarks.

What gets quoted is `benchstat before.txt after.txt` output, never a raw `ns/op` line: `ns/op` is a mean over one sample. `benchstat -col /engine` for the `engine=runner` and `engine=flatstack` subtests in `tests/flatstack/engine_compare_bench_test.go`, which are named for it.

There is no `b.ReportMetric` anywhere in the tree, so there is no custom metric to look for.

### Single threaded

`phpscript test` stays at its default `-p 1`. `--profile` cannot be combined with `-p` in any case, because Go's allocation counters are process-wide and cannot be attributed to one concurrent fixture.

### Stress runs

`--time=100ms` per sample. A one-second sample over 264 fixtures on two runners does not finish, and once a long run starts swapping it measures the swap.

### Memory

A measurement run has to be stopped from taking the box down. It is not a theoretical risk: a sampling run over the whole fixture suite in one process has been killed by the kernel, taking the rest of the sprint with it.

Three guards, all three required.

**`GOMEMLIMIT` on every measured process.** A ceiling well under what the box has free, 2 GiB by default:

```sh
GOMEMLIMIT=2GiB GOGC=100 ...
```

It is a soft limit, so the collector works harder as the heap approaches it instead of the kernel choosing a victim. That changes GC behaviour, and therefore timing, which is why both sides of a before/after pair carry the same `GOMEMLIMIT` and the manifest records it. A pair measured at two different limits is two measurements of two programs, the same way a CGO mismatch is.

**Scope a sampling run per area, not over the whole suite.** A runtime is retained for as long as its parse cache is, so a process that samples 264 fixtures holds 264 fixtures' worth of runtimes. One process per area releases it between areas:

```sh
for area in tests/fixtures/*/; do
  phpscript test --matrix --skip-php --profile --json \
    --count 6 --time 100ms "$area..." > "bench-fixtures-after-$(basename "$area").json"
done
```

**Know what `--cache` costs in memory, not only in what it measures.** `worker` keeps one set of caches per worker loop, which is the production shape and the larger heap. `off` drops the runtime with the fixture, which is far less resident and prices the parser instead of execution. A whole-suite run that has to stay in one process uses `off`.

`atkins bench` and `atkins default` are not exempt. Both compile and run the tree, and the OOM above was raised by atkins.

### GC and drift

Four rules. They were enforced by a script that has been deleted; they are protocol now.

- **Medians and percentiles, never means.** `--json` gives `p50_ns`, `p95_ns` and `p99_ns`; `benchstat` gives medians; `hey` and `wrk` give distributions. All three percentiles go in the artifact. p50 and p99 go in the table.
- **A static control that never reaches the interpreter.** For the HTTP target the control is `testdata/testserver.go`, the route-for-route Go twin. It is the floor the PHP numbers are read against, not a target to reach.
- **A drift guard across a segment.** Compare the first quartile of a segment's samples against the last and publish the percentage. A segment that drifted is re-measured, not published.
- **Drop a sample whose window overlapped a collection.** `--json` reports `gc_runs` per row. A row whose GC share differs between before and after is not comparable. Where a single request is the unit, the sample is dropped outright.

What does not come back is `GOGC=off` with a heap budget in the gigabytes. It is what made the old numbers exact, and it is also what takes the host down.

### Baselines and commits

Before and after come from the same worktree at two commits, both rebuilt from scratch, inside the same locked session where the run is short enough to allow it. `git stash` is not a baseline mechanism. The baseline is the merge-base, not whatever happened to be lying in the tree.

Every artifact set carries `bench-manifest-<side>.txt` holding the commit sha, `go version`, `CGO_ENABLED`, the binary path, `GOMAXPROCS`, the date, `uname -a` and the exact command lines. The manifest is where a CGO mismatch or a stale binary is caught before a number is published.

### Flag-parsing traps

- `-t` before a command name is `--testconfig` and runs instead of a command. After `test` it is `--time`. The same two letters, two meanings, decided by position.
- Global flags may be written before or after the command name.
- A directory path is not recursive. `phpscript test ./...` walks a tree; `phpscript test .` matches only the fixtures sitting directly in that directory and can pass over nothing. Every pipeline line spells the `./...`.

## The table shape

One shape per unit, one rule for before and after. [../allocation-performance.md](../allocation-performance.md) owns the allocation shape and is the worked example.

Allocation, as that document writes it:

| Return shape | B/op | allocs/op | ns/op |
|--------------|-----:|----------:|------:|

Latency, which no document carries yet:

| Fixture | p50 us | p99 us | allocs/op | B/op |
|---------|-------:|-------:|----------:|-----:|

HTTP, which no document carries yet:

| Route | p50 ms | p99 ms | req/s |
|-------|-------:|-------:|------:|

`--json` reports nanoseconds; the table reports microseconds, which is what `phpscript test` prints, to one decimal. p95 is collected and not published.

Before and after are `(was)` and `(now)` row labels, never extra columns. The environment is stated once above the table, in the form [../allocation-performance.md](../allocation-performance.md) uses: the harness, the box, the Go version and the CGO setting.

No document in this directory carries a measurement. A number in a prompt goes stale, and then the prompt lies. Numbers live in the pull request body, in the document that owns the subject, and in the gitignored artifact.

## Tests are not edited

Existing tests and fixtures pass unchanged. A passing test that starts failing is a regression in the change, not a stale test.

No test, no fixture, no expected-output section and no allocation budget is edited to accommodate new behaviour. Deleting one is the same act as editing it: a test is never dead code.

The only permitted edit is one the operator has confirmed after being shown four things: the test as it stands, the output it now produces, what `php` prints for the same source where it is a `.phpt`, and which of the two is wrong.

The escape hatch is exact. The agent stops. It does not edit the test, skip it, mark it expected-fail, comment it out, or raise a budget. It writes the four items and asks. Other findings continue; that one blocks.

A new fixture's expected section is produced by running the source through real `php` and pasting the output, never from phpscript's own. [../testing.md](../testing.md) owns the format and the oracle rule.

### The guards

Three tests fail the build rather than skipping when they regress.

| Guard                                      | Where                          | What it holds                                                                         |
|--------------------------------------------|--------------------------------|---------------------------------------------------------------------------------------|
| `TestFlatstackPrecompiledAllocationBudget` | `tests/flatstack/load_test.go` | The concat, expression-loop and user-function budgets, through `testing.AllocsPerRun` |
| `TestLexOperatorNoAlloc`                   | `parser/lexer_test.go`         | The lexer's operator path allocates nothing                                           |
| `TestNoInheritanceAtRuntime`               | `runner`                       | `model.Class` has no `Parent` field                                                   |

A budget is lowered in the same commit as the improvement that lowers it, the way `scripts/splint-baseline.json` is lowered. A budget is never raised. A change that needs one raised goes to the operator. `TestNoInheritanceAtRuntime` is a design guard and is not negotiable at all.

## The config model is frozen

No change adds, renames, retypes or removes a yaml-tagged field under `config/`.

`config.Config` embeds `runner.Options` as its `runner:` block, so every yaml-tagged field of `runner.Options` is inside the same model and under the same freeze.

Defaults are part of the model. `config/config.yml` and the `Resolved` and `Validate` paths decide what an existing file means, so changing a default changes behaviour for files nobody edited.

Reading an existing field from a new place is allowed. Where a diagnosis needs a config change, the change is not made: the pull request carries the extended reasoning below and the operator decides.

## Techniques

Three tiers, and one rule that applies to all of them.

**Tier 1, measurement is the whole justification.** Removing an allocation nothing reads. Presizing. Reusing a backing array instead of dropping and regrowing it. Hoisting per-process work out of the per-request path. Building lazily what most requests never read. Pooling a value whose lifetime ends at the request boundary.

**Tier 2, allowed with an argued reason in the pull request and a fixture.** Immutability: sharing a value instead of copying it. The argument names what guarantees nobody writes to it; the fixture is the case where two requests hold it at once. The repo already leans this way - a handler's captures are shared across requests, and flatstack's bytecode is immutable and shared by program identity.

**Tier 3, needs the operator before it is written, not after.** `unsafe` in any form. `unsafe.String` or `unsafe.Slice` over a buffer a caller may still write. Pointer rewriting of model values. A free list whose entries outlive a request. Any change to value semantics. These are the changes whose failure mode is a wrong answer under concurrency rather than a slow one, and the suite is not a proof of their absence. Each needs what aliases what, which test holds it, and a `-race` run over the affected packages.

**All three tiers.** A technique is explored by measuring it. A technique that was measured and lost is recorded, with its number and why it lost. Rejected is a result: `../flatstack.md` records the typed-slot verdict and `../allocation-performance.md` records two reverted shapes, and both issue templates carry a considered-and-rejected field.

## The pull request

Body order, fixed:

1. The summary statement, first line. For a positive change, this shape: `Optimized execution of expressions by changing the behaviour from _ to _. This nets a positive change of -X% in latency and a delta from X allocs/op to Y allocs/op (-30%).` The percentage is a signed delta; negative is the improvement for latency and allocations. An absolute number is never the claim. Both sides come from one protocol run.
2. The problem-first body. `type(scope): subject` title, no line wrapping, no test walkthrough, no toolchain notes.
3. The before/after table, canonical shape, environment stated once.
4. The `benchstat` block verbatim for whatever moved.
5. Considered and rejected: at least one entry, each naming the approach, the number it produced and why it lost. Where a future reader would try it again, the same sentence also lands in the document that owns the subject, because a pull request body is not where anybody looks.
6. `worktree verdict --from=main` output, that exact command, last. It is already markdown - a heading, a verdict line and a table - so it goes in raw rather than fenced.
7. The checklist from `.github/PULL_REQUEST_TEMPLATE.md`, ticked honestly.

### Extended reasoning for a possible breaking change

Six slots, one or two lines each.

| Slot                 | What it says                                                                                                                        |
|----------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| Cost                 | The call, the measured number, and how it was measured                                                                              |
| What the field buys  | The behaviour that would be lost                                                                                                    |
| Consumer and trigger | Who it serves, what triggers it, and why those two do not match                                                                     |
| Intent               | The stated purpose the current trigger is wrong against                                                                             |
| Proposal             | Where the setting lives, its name, its default, and what an existing file does with the key absent                                  |
| What breaks for whom | The config keys, demo, fixture or venom suite whose behaviour changes, and what a user who upgrades without editing their file sees |

The shape, as the operator writes it:

> Calling runtime.ReadMem is a 400ns performance hit; the field is used to provide \_\_\_\_, and it does so for the service level runtime, however is triggered by a http request. As the command is intended for server monitoring, the behaviour changes to be configured in the config package, adds a monitoring interval to trigger it in the background.

## Artifacts

The naming rule is the gitignore rule. Every name below is already covered by a line in `.gitignore`, which is why collecting a measurement needs no change to it.

| Artifact         | Name                                     | Already ignored by  |
|------------------|------------------------------------------|---------------------|
| benchstat input  | `bench-go-<side>.txt`                    | `bench*.txt`        |
| fixture sampling | `bench-fixtures-<side>.json`             | `bench*.json`       |
| `go test` run    | `bench-gotest-<side>.json`               | `bench*.json`       |
| load generator   | `bench-http-<side>.txt`                  | `bench*.txt`        |
| provenance       | `bench-manifest-<side>.txt`              | `bench*.txt`        |
| pprof            | `cpu-<label>.pprof`, `mem-<label>.pprof` | `*.pprof`           |
| coverage         | `cover/<label>.cov`                      | `/cover/*`, `*.cov` |

Raw output lives at the worktree root, or under `cover/` for coverage profiles, never under `docs/`. Any other extension is forbidden, so that those lines keep covering it.

The repo root already holds `bench-after.txt`, `bench-after2.txt`, `bench-after3.txt`, `bench-before.txt`, `bench-e2e-*.json` and `bench-profile-*.json` from earlier sprints, with no manifest and unknown build provenance. They are not baselines. The `flatstack.test`, `runner.test` and `tests.test` binaries beside them are ignored leftovers, not a build of this tree.

Curated output goes in the pull request body and, where the number is durable, in the document that owns the subject: binding shapes to [../allocation-performance.md](../allocation-performance.md), engine comparison to [../flatstack.md](../flatstack.md). Nothing timing-dependent is committed. `atkins.yml` already records why, on `test:phpscript:matrix`: the fixture report is generated deliberately without `--profile`, because a timing that differs by a millisecond per run would be a diff in every commit.

## Decisions that do not come back

`scripts/memprobe/` is deleted. What it got right survives above as protocol: the two memory numbers, medians and percentiles over means, the static control route, the drift guard, and dropping a sample that overlapped a collection.

What does not come back, each with its reason:

- `GOGC=off` with `GOMEMLIMIT=off` and a heap budget in the gigabytes. It made a single sequential request an exact measurement, and it takes the host down.
- A shell harness that restarts a server per segment. The restart is where the provenance gets lost.
- `taskset -c 3` over a parallel benchmark. It measures one goroutine and reports it as concurrency.
