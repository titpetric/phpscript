# The pipeline

`atkins` runs the jobs of [`atkins.yml`](../atkins.yml). A job's name and `desc` state its purpose. This document holds the reasoning behind the order, the splits and the generation. `atkins.yml` writes one comment above each top-level key. A nested key, a list item, a value take none.

`atkins --final default` is the gate. It reads a change: the formatter, the linters, the Go suite, the generated pages and the image. The file sets `GOFLAGS: ""` at the top: a `-mod=mod` inherited from a go.work environment breaks every go command.

## What the default pipeline runs

| Order | Job                  | What it settles                                  |
|-------|----------------------|--------------------------------------------------|
| 1     | `fmt`                | the Go source and the markdown, in one notion    |
| 2     | `lint`               | splint, bindlint, the vale gate, mdox            |
| 3     | `go:test`            | the Go suite, writing `cover/pkg.cov`            |
| 4     | `go install .`       | the binary the generated pages read              |
| 5     | `test:introspection` | the three generated reference pages, regenerated |
| 6     | `fmt`                | the documentation step 5 wrote                   |
| 7     | `bake`               | the image for both architectures, into the cache |

`fmt` runs twice: ahead of the test, then over the generated output. Every formatting job calls `fmt`, never `go:fmt` or `mdox:fmt` alone. The two tools then cannot drift apart. `lint` applies that notion across the linters.

`go install .` is the one command in `default` that names no task. `test:introspection` runs `phpscript info` and needs a binary that matches the tree, because the released one may predate the change. It leaves nothing behind but the binary.

## What ci:daily runs

`ci:daily` runs the suites a push cannot: `deploy` first to publish the image, then `db:up`, `test:phpscript:run`, `test:phpscript:matrix`, `test:lifecycle` and `test:demos`. The pipeline declares the name a scheduler looks for, while the schedule itself lives outside the repository. The default gate never starts a docker service, a second runtime or a server process.

`bake` owns the image. It builds `docker/Dockerfile.build` for `linux/amd64` and `linux/arm64`, tagged `titpetric/phpscript:latest`, a tag that follows `main` and is unstable. `atkins deploy` publishes it. A release sets `TAG` before that push. `ci:daily` publishes it before the integration legs.

The first run creates the `docker-container` builder `phpscript`, because the default driver pushes a single platform. `bake` runs one `docker buildx bake` that builds both platforms into the builder's cache, and `atkins deploy` pushes `latest-amd64` and `latest-arm64` as separate images. A source change reuses the Go module and compile caches through the mounts in the Dockerfile.

The demo suites stay out of `default`. They are compose services. Running them costs the whole stack plus a venom suite per demo, which answers a question about the demo applications, not about the runtime. `atkins test:demos` runs them against the image `ci:daily` built. A change under `demos/` runs it by name.

`mysql` and `postgres` are test infrastructure, outside the demos. `test-database.php` and the `platform_database` fixtures query all three dialects, and sqlite alone would miss a dialect divergence. `ci:daily` starts both, and tears them down when the run ends.

## The jobs outside the default pipeline

`setup` installs the two tools `atkins.yml` calls by name, `summary` and `splint`, at the revision their repositories hold. The repository changes both, and neither changes for it alone. A tag would lag a fix made for a job here. `mdox` stays out: `atkins mdox:setup` declares `interactive: true` and fails without a tty.

`bench` delegates to two jobs. `taskset -c 3` narrows the affinity mask to one core. Go reads that mask for `GOMAXPROCS`, leaving a `b.RunParallel` benchmark there to run a single goroutine: a contention fix measures as zero, a contention regression as nothing. `bench:pinned` skips three parallel benchmarks. `bench:parallel` runs them unpinned, at the higher `-count`: a parallel benchmark on a shared box is the noisiest number in the set. `BenchmarkLookupTreeSize` is serial, and the `$` anchoring the first selector name keeps it in the pinned job.

Both halves nest their variables in `env.vars`. A flat `env` block reaches the command line, not the child process: `$PARALLEL` expands to nothing, the pinned job skips nothing and runs three parallel benchmarks as a single goroutine, the parallel job matches nothing and runs none, and `GOMEMLIMIT`, `GOGC`, `CGO_ENABLED` never reach `go test`.

A bench sweep costs minutes of pinned CPU and answers a performance question, which keeps it out of `default`. Durable numbers land as curated tables in [Allocation and performance](allocation-performance.md), and the `bench*.txt` files are in `.gitignore`. [The measurement protocol](agents/README.md#the-measurement-protocol) governs both halves.

`publish` shadows the `publish` alias the release skill contributes. `atkins publish` attaches everything a release holds, the binaries included. `release:publish` keeps its name. Run `atkins bake` and `atkins gen` first: the release skill creates a release for an existing tag and never creates the tag. `bin/` plus five generated documents are in `.gitignore` for the same reason.

The shared release skill builds through docker and names one tag. `release:notes` and `release:binaries` read git instead: every tag without a release, built from its source with `go build` alone. `scripts/release.sh --help` states the flags.

## Generation

Three generated files are in git. `test:introspection` rewrites them in the default pipeline. Hand-written reference pages link them, and `docs/reference/README.yml` registers them.

| File                                            | Written by                |
|-------------------------------------------------|---------------------------|
| `docs/reference/README.md`                      | `php README.php`          |
| `docs/reference/extensions/implemented-apis.md` | `scripts/list-apis`       |
| `docs/reference/extensions/go-imports.md`       | `scripts/docs-go-imports` |

The two listings read the runtime the build binds. `scripts/docs-go-imports` renders the rows `phpscript list --runtime=go --bindings --json` prints, with the prose around them in `go-imports.md.tpl`, next to the page.

The index comes first, the markdown second. The index is a Go file the next build embeds: a shipped binary holds no source tree, and `phpscript list --classes` answers out of the index. `scripts/list-apis --index` writes that file and takes no redirection, which would truncate the bytes the compiler is about to embed. The job ends with `mdox:fmt`. Each generator emits an unpadded table, and the formatter repads it, leaving a generated file dirty after every run.

`.gitignore` names five more. They travel as release assets: each changes on every run, and generating them inside a correctness run dirtied the tree on every invocation.

| File                                | Written by                    | Reads                     |
|-------------------------------------|-------------------------------|---------------------------|
| `docs/test-fixtures.md`             | `atkins gen:fixtures`         | a matrix run of the suite |
| `docs/coverage/phpscript.md`        | `atkins cover`                | `cover/pkg.cov`           |
| `docs/coverage/phpscript-detail.md` | `atkins cover`                | `cover/pkg.cov`           |
| `docs/coverage/dbadmin.md`          | `atkins gen:coverage:dbadmin` | `cover/dbadmin.file.txt`  |
| `docs/coverage/dbadmin-detail.md`   | `atkins gen:coverage:dbadmin` | `cover/dbadmin.file.txt`  |

`atkins gen` writes all five, `atkins release:docs` attaches them, and `atkins publish` runs both halves. Do not commit one.

`gen:fixtures` is the matrix run again with `-o`, a second execution of the suite. The gate runs on every pipeline invocation and this job does not. Paying for the report on demand is the cheaper half. The command names no `--profile`, `--count` or `--time`: a timing that differs by a millisecond per run would dirty every commit.

The Go coverage pair reads the `cover/pkg.cov` that `go:test` wrote, through the `.md.tpl` template beside each document. `splint coverage` answers this and `summary coverfunc` does not. splint reads the Go source alongside the profile, which lets a row hold the cognitive complexity and the line count next to the percentage, sorted, in markdown tables. Two documents: a reader scans the per-package summary, while the per-function detail serves somebody hunting the gap. Each is the same command with the other template.

The dbadmin pair stays on `summary coverfunc`. Its input is PHP coverage read off the running service, and splint parses Go source. `cover/dbadmin.file.txt` comes from the dbadmin venom suite in `demos/dbadmin/tests/atkins.yml`, which reads it off the running container. The report then states which paths an end-to-end run of the application executed.

A tree without a measurement leaves the documents alone and writes no empty report. The coverage pair needs `atkins go:test` and the dbadmin pair needs `atkins test:demos`. Each names the missing input and exits 0.

`atkins gen` ends with `fmt`. splint and mdox disagree on how wide an emoji is, the coverage tables come out of splint with padding mdox rewrites, and without that pass the target always leaves a diff. `atkins cover` alone runs no `fmt`. Run it by hand and leave the padding alone.

Each release asset keeps its basename. `docs/coverage/phpscript.md` links `phpscript-detail.md` by a bare filename, and gh serves a tag's assets from one flat directory. The link resolves between two assets by the names this table records. The part after `#` in an `assets` entry is gh's display label, which keeps the list readable, and renames nothing.

## The linters

`splint ./...` runs bare, over all rules at all severities: one finding fails the run. It had a ratchet over `scripts/splint-baseline.json` until the tree reached zero. A baseline records the findings of the commit that writes it: eighteen files over the length limit stayed there while nobody lowered the number, and the count drifted above the recorded number without failing anything. The target for each rule is zero.

vale keeps its gate: its exit status reads a rule's level alone. Three write-good rules at `level: warning` printed 4,606 findings and passed. [`scripts/vale-gate.sh`](../scripts/vale-gate.sh) states the reasoning. Its baseline holds no rule at all.

`bindlint` is the house rule splint cannot see. A binding that points at `*model.Array` or `*model.Object` answers for the values the interpreter built and refuses the Go slice, map or struct another binding returned. [Allocation and performance](allocation-performance.md) states the rules it reads.

`lint` runs after the first `fmt`. mdox then checks markdown the formatter has already settled. A finding there is prose, not padding. It runs ahead of the test legs: a prose finding costs seconds to fix, the suite costs minutes to reach.

## Fixture paths

`phpscript test ./...` runs a tree. `phpscript test .` matches only the fixtures directly in the directory and can silently pass. Every job spells the `./...`: a job that omits it points at a directory of folders and passes over nothing.

`test:phpscript:matrix` runs every fixture on all three runtimes. A divergence between the two Go runtimes, or between phpscript and php itself, fails the pipeline. The job reports the suite and writes no file.

`test:lifecycle` drives the server like an operator: a real process, a real pidfile, a real signal. No fixture reaches any of that. Two parts matter most: the pidfile names the running process, and a refused reload leaves it serving. A Go test in the same process cannot reach either.

## Composer

`titpetric/minitpl` arrives per project and nobody vendors it by hand. Every tree with a `composer.json` installs before the demo suites run. No Go test reads these trees, which keeps `composer:install` out of the default pipeline: `test:demos`, the job that executes them, owns it.

A `for:` over `demos.all` replaces the shell loop. The shell loop took the exit code of its last iteration: a failed install in `demos/example` passed the task as long as the next one worked.

`demos.all` names every tree with a `composer.json`. `demos.venom` is the subset that is a compose service with a separate venom suite. `demos/common` joins the first, not the second: that package's fixtures share a directory with its source, which answers to `test:phpscript:common`, never to a server.

Each demo declares a separate pipeline, because it resolves the address of its container first. `test:demos:venom` delegates to `atkins test` in the demo's `tests/` and runs no venom here.

## The demo pipelines

Neither demo service publishes a port. `demos/dbadmin/tests/atkins.yml` reads its container's address out of `docker inspect`, then passes it to venom through `--var=host=`. The example pipeline does the same. `atkins host` prints that address alone, for a run by hand.

Each lookup is a pipeline of `xargs --no-run-if-empty`. atkins expands `$(...)` as it reads the file, ahead of bash: a command substitution would resolve while the file loads, before any container runs. The `--no-run-if-empty` guard holds back the stage after it when `docker compose ps -q` prints no id, and `atkins host` then fails with a message naming `atkins up`.

Each suite restarts its service first. The container reads the PHP sources through a volume mount. A restart re-reads an edited one.

## The compose services

[`compose.yml`](../compose.yml) runs both demos off the image `build` wrote, and includes the mysql and postgres services from `docker/service/`.

The dbadmin service takes `--cover`: the venom suite in `demos/dbadmin/tests` measures those PHP sources. Every request folds into a single aggregator. `/debug/phpscript/coverage` publishes it while the container runs, and shutdown writes `cover/dbadmin.{time}.cov` into the mounted `cover/`. `{time}` names one file per run and keeps the file the run before it wrote.

That service joins two networks. caddy reaches it over the ingress network, while the compose default hosts mysql and postgres. dbadmin opens a connection to each dialect. A service that cannot resolve them reports every driver but sqlite as unreachable.

The example service keeps a `config.yml` and names it with `-f`. `.env.docker` configures the dbadmin service alone.

## The vendored skills

`.atkins/skills/` holds this repository's copies of the shared skills. `go.yml` and `mdox.yml` differ from the ones in `$HOME` in one path: every `.cov` file this tree produces goes under `cover/`, Go and PHP alike, which puts the coverage artifact of a run in one directory. The other repositories read `pkg.cov` at their root.

`go:test` puts the package list last. gotestsum passes everything after `--` to go test, and `-coverprofile` writes no profile, silently, when the list precedes the flags.

`go:fmt` runs `splint fix ./...` for one pass across the source data model: every import block grouped and ordered, an import nothing reaches taken out, an import a name needs put in. `go mod tidy` follows it. `mdox:fmt` passes `--markdown`, which keeps the fixer off the Go: the import blocks answer to `go:fmt`, and a task named for the documents rewrites none of them.

`mdox:lint` checks no formatting. The formatter settles a heading level, a list marker, a run of spaces, and takes no configuration. A check would report a file nobody had run the formatter over. vale runs from the repository root, where it finds `.vale.ini` and `.vale/styles`. The root also reaches the Go doc comments, which answer to the same prose rules as a document. `scripts/vale-gate.sh` runs where the repository holds one, and a bare `vale .` runs elsewhere.

`mdox:prose` reports the constructions this tree leans on that the standard library does not. The first run measures the standard library and keeps the counts, at a cost of about twenty seconds. The rest are instant. The rule reports warnings and prints a table, leaving a reader to decide what to change.
