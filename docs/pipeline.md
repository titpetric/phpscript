# The pipeline

`atkins` runs the jobs of [`atkins.yml`](../atkins.yml). A job's name and its `desc` say what it does. This document holds the reasoning behind the order, the splits and the generation. `atkins.yml` writes one comment above each top-level key and leaves a nested key, a list item and a value with nothing.

`atkins --final default` is the gate. The file sets `GOFLAGS: ""` at the top because a `-mod=mod` inherited from a go.work environment breaks every go command.

## What the default pipeline runs

| Order | Job                     | What it settles                                       |
|-------|-------------------------|-------------------------------------------------------|
| 1     | `fmt`                   | the Go source and the markdown, in one notion         |
| 2     | `lint`                  | splint, bindlint, the vale gate, mdox                 |
| 3     | `go:test`               | the Go suite, writing `cover/pkg.cov`                 |
| 4     | `build`                 | the release binary and the docker image               |
| 5     | `db:up`                 | the mysql and postgres services, torn down at the end |
| 6     | `test:phpscript:run`    | the four scripts a release has to be able to run      |
| 7     | `test:phpscript:matrix` | every fixture on all three runtimes                   |
| 8     | `test:lifecycle`        | the server's pidfile, its reload and its refusal      |
| 9     | `test:introspection`    | the two generated reference pages, regenerated        |
| 10    | `fmt`                   | the documentation step 9 wrote                        |

`fmt` runs twice: once ahead of the build, once over what the generation wrote. Every job that formats delegates to it and never to `go:fmt` or `mdox:fmt` alone. The two tools then cannot drift apart, and `lint` is the same notion for the three linters.

`docker:build` belongs to `build` and not to the demos. The image is what `compose:up` and `compose:down` operate on and what a deployment ships. A pipeline run leaves a current one behind for a deployment that never touches the demos.

The demo suites stay out of `default`. They are compose services, and running them costs the whole stack plus a venom suite per demo, which answers a question about the demo applications and never one about the runtime. `atkins test:demos` runs them against the image the pipeline built, and a change under `demos/` runs it.

`mysql` and `postgres` are test infrastructure and no part of the demos. `test-database.php` and the `platform_database` fixtures query all three dialects, and sqlite alone would miss a dialect divergence. Both publish a port to the host and come up without the image the demos need.

## The jobs outside the default pipeline

`setup` installs the two tools `atkins.yml` calls by name, `summary` and `splint`, at the revision their repositories hold. Both change under this repository and never for it. A tag would lag a fix made for a job here. `mdox` stays out: `atkins mdox:setup` declares `interactive: true` and fails with no tty.

`bench` delegates to two jobs. `taskset -c 3` narrows the affinity mask to one core, Go reads that mask for `GOMAXPROCS`, and a `b.RunParallel` benchmark under it then runs a single goroutine: a contention fix measures as zero and a contention regression measures as nothing. `bench:pinned` skips the three parallel benchmarks and `bench:parallel` runs them unpinned, at the higher `-count`, because a parallel benchmark on a shared box is the noisiest number in the set. `BenchmarkLookupTreeSize` is serial and belongs to the pinned job, which the `$` anchoring the first name in the selector keeps it in.

Both halves nest their variables under `env.vars`. A flat `env` block reaches neither the command line nor the child process: `$PARALLEL` expands to nothing, the pinned job skips nothing and runs the three parallel benchmarks on one goroutine, the parallel job matches nothing and runs none, and `GOMEMLIMIT`, `GOGC` and `CGO_ENABLED` never reach `go test`.

A bench sweep costs minutes of pinned CPU and answers a performance question, which keeps it out of `default`. Durable numbers land as curated tables in [Allocation and performance](allocation-performance.md); the `bench*.txt` files are in `.gitignore`. [The measurement protocol](agents/README.md#the-measurement-protocol) governs both halves.

`publish` shadows the `publish` alias the release skill contributes. `atkins publish` attaches everything a release holds and never the binaries alone, and `release:publish` stays reachable under its own name. Run `atkins build` and `atkins gen` first: the release skill creates a release for an existing tag and never the tag itself, `bin/` is in `.gitignore`, and the five generated documents are there for the same reason.

## Generation

Two generated files are in git, and `test:introspection` rewrites them in the default pipeline. Hand-written reference pages link them and `docs/reference/README.yml` registers them.

| File                                            | Written by          |
|-------------------------------------------------|---------------------|
| `docs/reference/README.md`                      | `php README.php`    |
| `docs/reference/extensions/implemented-apis.md` | `scripts/list-apis` |

The index comes first and the markdown second. The index is a Go file the next build embeds: a shipped binary holds no source tree and `phpscript list --classes` answers out of the index. `scripts/list-apis --index` writes that file itself and takes no redirection, which would truncate what the compiler is about to embed. The job ends with `mdox:fmt`. Each generator emits an unpadded table and the formatter repads it, which left both files dirty after every run.

`.gitignore` names five more. They travel as release assets, because each one changes on every run and generating them inside a correctness run dirtied the tree on every invocation.

| File                                | Written by                    | Reads                     |
|-------------------------------------|-------------------------------|---------------------------|
| `docs/test-fixtures.md`             | `atkins gen:fixtures`         | a matrix run of the suite |
| `docs/coverage/phpscript.md`        | `atkins cover`                | `cover/pkg.cov`           |
| `docs/coverage/phpscript-detail.md` | `atkins cover`                | `cover/pkg.cov`           |
| `docs/coverage/dbadmin.md`          | `atkins gen:coverage:dbadmin` | `cover/dbadmin.file.txt`  |
| `docs/coverage/dbadmin-detail.md`   | `atkins gen:coverage:dbadmin` | `cover/dbadmin.file.txt`  |

`atkins gen` writes all five, `atkins release:docs` attaches them, and `atkins publish` does both halves. Do not commit one.

`gen:fixtures` is the matrix run again with `-o`, a second execution of the suite. The gate runs on every pipeline invocation and this job does not, and paying for the report on demand is the cheaper half. The command names no `--profile`, `--count` or `--time`: a timing that differs by a millisecond per run would be a diff in every commit.

The Go coverage pair reads the `cover/pkg.cov` that `go:test` wrote, through the `.md.tpl` template beside each document. `splint coverage` answers this and `summary coverfunc` does not: splint reads the Go source alongside the profile, and a row then holds the cognitive complexity and the line count next to the percentage, sorted, in markdown tables. Two documents, because the per-package summary is what a reader scans and the per-function detail is what somebody hunting the gap reads. Each is the same command with the other template.

The dbadmin pair stays on `summary coverfunc`. Its input is PHP coverage read off the running service, and splint parses Go source. `cover/dbadmin.file.txt` comes from the dbadmin venom suite in `demos/dbadmin/tests/atkins.yml`, which reads it off the running container. The report then states what an end to end run of the application executed.

A tree with no measurement yet leaves the documents alone and writes no empty report. The coverage pair needs `atkins go:test` and the dbadmin pair needs `atkins test:demos`. Each names the missing input and exits 0.

`atkins gen` ends with `fmt`. splint and mdox disagree on how wide an emoji is, the coverage tables come out of splint with padding mdox rewrites, and without that pass the target always leaves a diff. `atkins cover` on its own runs no `fmt`. Run it by hand and leave the padding alone.

Each release asset keeps its own basename. `docs/coverage/phpscript.md` links `phpscript-detail.md` by a bare filename and gh serves a tag's assets from one flat directory. The link resolves between two assets under the names this table records. The part after `#` in an `assets` entry is gh's display label, which makes the list readable and renames nothing.

## The linters

`splint ./...` runs bare, over every rule at every severity, and one finding fails the run. It had a ratchet over `scripts/splint-baseline.json` until the tree reached zero. A baseline records the findings of the commit that writes it: the eighteen files over the length limit stayed in it for as long as nobody lowered the number, and the count drifted above the recorded one without failing anything. The target for every rule is zero.

vale keeps its gate, because its exit status reads a rule's level and no count: the three write-good rules at `level: warning` printed 4,606 findings and passed. [`scripts/vale-gate.sh`](../scripts/vale-gate.sh) states the reasoning. Its baseline holds no rule at all.

`bindlint` is the house rule splint cannot see. A binding that points at `*model.Array` or `*model.Object` answers for the values the interpreter built and refuses the Go slice, map or struct another binding returned. [Allocation and performance](allocation-performance.md) states the rules it reads.

`lint` runs after the first `fmt`. mdox then checks markdown the formatter has already settled, and a finding there is prose and never padding. It runs ahead of the test legs, because a prose finding costs seconds to fix and the suite costs minutes to reach.

## Fixture paths

`phpscript test ./...` runs a tree. `phpscript test .` matches only the fixtures directly in the directory and can silently pass. Every job spells the `./...`: without it a job points at a directory of folders and passes over nothing.

`test:phpscript:matrix` runs every fixture on all three runtimes, and a divergence between the two Go runtimes, or between phpscript and php itself, fails the pipeline. The job reports the suite and writes no file.

`test:lifecycle` drives the server the way an operator drives it: a real process, a real pidfile and a real signal. No fixture reaches any of it, and the two parts that matter most, that the pidfile names the running process and that a refused reload leaves it serving, are out of reach of a Go test in the same process too.

## Composer

`titpetric/minitpl` arrives per project and nobody vendors it by hand. Every tree with a `composer.json` installs before the demo suites run. No Go test reads these trees, which keeps `composer:install` out of the default pipeline: `test:demos`, the job that executes them, owns it.

The job is a `for:` over `demos.all` and no shell loop. The shell one took the exit code of its last iteration, and a failed install in `demos/example` passed the task as long as the one after it worked.

`demos.all` is every tree with a `composer.json`. `demos.venom` is the subset that is a compose service with a separate venom suite. `demos/common` joins the first and not the second: a package whose fixtures share a directory with its source, which answers to `test:phpscript:common` and to no server.

Each demo declares a separate pipeline, because it resolves the address of its container first. `test:demos:venom` delegates to `atkins test` in the demo's `tests/` and runs no venom here.

## The demo pipelines

Neither demo service publishes a port. `demos/dbadmin/tests/atkins.yml` and `demos/example/tests/atkins.yml` each read their own container's address out of `docker inspect` and pass it to venom as `--var=host=`. `atkins host` prints that address alone, for a run by hand.

Each lookup is a pipeline of `xargs --no-run-if-empty`. atkins expands `$(...)` as it reads the file, ahead of bash: a command substitution would resolve while the file loads, against a container nobody has started. The `--no-run-if-empty` guard holds back the stage after it when `docker compose ps -q` prints no id, and `atkins host` then fails with a message naming `atkins up`.

Each suite restarts its service first. The container reads the PHP sources through a volume mount. A restart re-reads an edited one.

## The compose services

[`compose.yml`](../compose.yml) runs both demos off the image `build` wrote, and includes the mysql and postgres services from `docker/service/`.

The dbadmin service runs under `--cover`, because the venom suite in `demos/dbadmin/tests` is what measures those PHP sources. Every request folds into one aggregator. `/debug/phpscript/coverage` publishes it while the container runs, and shutdown writes `cover/dbadmin.{time}.cov` into the mounted `cover/`. `{time}` names one file per run and keeps the file the run before it wrote.

That service joins two networks. caddy reaches it over the ingress one, and the compose default is where mysql and postgres listen. dbadmin opens a connection to each dialect, and a service that cannot resolve them reports every driver but sqlite as unreachable.

The example service holds its own `config.yml` and names it with `-f`. `.env.docker` configures the dbadmin service alone.

## The vendored skills

`.atkins/skills/` holds this repository's copies of the shared skills. `go.yml` and `mdox.yml` differ from the ones in `$HOME` over one path: every `.cov` file this tree produces goes under `cover/`, Go and PHP alike, and one directory is then the coverage artifact of a run. The other repositories read `pkg.cov` at their root.

`go:test` puts the package list last. gotestsum passes everything after `--` to go test, and `-coverprofile` writes no profile and no word of complaint when the list precedes the flags.

`go:fmt` runs `splint fix ./...` for one pass over a data model of the source: every import block grouped and ordered, an import nothing reaches taken out, an import a name needs put in. `go mod tidy` follows it. `mdox:fmt` passes `--markdown`, which keeps the fixer off the Go: the import blocks answer to `go:fmt`, and a task named for the documents rewrites none of them.

`mdox:lint` checks no formatting. The formatter settles a heading level, a list marker and a run of spaces with no rule to configure. A check would report a file nobody had run the formatter over. vale runs from the repository root, where it finds `.vale.ini` and `.vale/styles`, and the root also reaches the Go doc comments, which answer to the same prose rules as a document. `scripts/vale-gate.sh` runs where the repository has one, and a bare `vale .` where it does not.

`mdox:prose` reports the constructions this tree leans on that the standard library does not. The first run measures the standard library and keeps the counts, which costs about twenty seconds; the rest are instant. The rule reports warnings and prints a table; a reader decides what to change.
