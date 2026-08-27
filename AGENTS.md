# AGENTS.md - phpscript repo

Portable instructions for AI coding agents. Keep this file lean; the reasoning lives in `docs/`. Nothing here restates a rule at length: it names the rule, the guard that enforces it, and the document that argues it.

## Non-negotiable

These are decisions, not gaps. A prompt asking for one of them gets the pointer, not the feature.

- **No object model beyond namespaced bags of properties and methods.** `extends` is parsed and recorded so the formatter and linter can see it; nothing in `runner` may read it. `runner.TestNoInheritanceAtRuntime` fails the build if `model.Class` regrows a `Parent` field. No traits, no `parent::`, no magic methods beyond `__construct` and `__invoke`. [docs/design.md](docs/design.md)
- **The "Won't implement" table in design.md is a decision, not a backlog.** `eval`, `goto`, generators, `curl_*`, PDO, `${var}` interpolation, `&` outside `foreach`, and the rest each name what to use instead. Separate from "Not implemented" in the language reference, which means not yet.
- **The root level is closed.** New code goes into an existing package or a subdirectory of the package that owns the area (`stdlib/<area>`, `flatstack/engine`, `cmd/phpscript/<command>`). `parser` and `runner` do not import each other; they meet in `model`. An area not in the areas-of-interest table is out of scope and goes through an issue first. [docs/naming-conventions.md](docs/naming-conventions.md)
- **A PHP name is a behaviour claim, settled by php itself.** Write the fixture by running the source through `php` first and pasting that output as the expected section, then make phpscript produce it. Expected output written from phpscript's own print locks in its bugs. [docs/testing.md](docs/testing.md)
- **Binding return shapes follow the allocation rules.** Arguments arrive as `any`, never `*model.Array` (a concrete parameter makes `reflect.Call` panic on a binding's `[]string`). Return the Go value you already have; `*model.Array` only for the three listed reasons. `runner/compile_test.go::TestCompileMatchesExprEnv` guards the expr environment. [docs/allocation-performance.md](docs/allocation-performance.md)
- **Flatstack fallback is atomic.** The whole program compiles or the whole program delegates to the runner; partial execution would repeat side effects. Benchmarks gate on `flatstack.Supports`; applications do not check it. [docs/flatstack.md](docs/flatstack.md)

## Working here

- `go install .` before testing a change. The `$PATH` binary is a published build and does not contain your edits.
- Fixture paths: `phpscript test ./...` runs a tree; `phpscript test .` matches only fixtures directly in the directory and can silently pass. The `./...` is load-bearing in every pipeline job.
- A change to language or runtime behaviour lands with a `.phpt` fixture in the matching `tests/fixtures/<area>/` folder. A new area is a new folder; nothing registers it. The fixture's own folder is its include root; copy shared support files rather than climbing out.
- Pre-submit: the affected package tests, then `go test ./...`, then `phpscript test --matrix tests/fixtures/...`. A fixture covering PHP behaviour is not done until its php column passes.
- Generated files are regenerated, never hand-edited: `docs/reference/README.md` (from `php README.php`), `docs/reference/extensions/implemented-apis.md` (from `scripts/list-apis`), `docs/test-fixtures.md` (from `phpscript test --matrix -o`, deliberately without `--profile`). A new doc under `docs/reference/` must be listed in `docs/reference/README.yml` or `README.php` throws.
- Registrations carry a one-line doc comment starting with the name a script types; the generated reference is built from it. Renaming a registered class registers the new name first and keeps the old one as a second registration; removal is a separate change.
- The pipeline is `atkins` (`atkins --final default`). It sets `GOFLAGS: ""` because a `-mod=mod` inherited from a go.work environment breaks every go command.

## Load order

| Read when | Document |
|-----------|----------|
| Proposing or rejecting a language feature | [docs/design.md](docs/design.md) |
| Adding or renaming anything script-visible | [docs/naming-conventions.md](docs/naming-conventions.md) |
| Writing or fixing a fixture | [docs/testing.md](docs/testing.md) |
| Writing or changing a Go binding | [docs/allocation-performance.md](docs/allocation-performance.md), [docs/reference/extensions/bindings.md](docs/reference/extensions/bindings.md) |
| Touching the bytecode engine | [docs/flatstack.md](docs/flatstack.md) |
| Explaining output that differs from php | Known divergences in [docs/README.md](docs/README.md) |

## Checkouts

This checkout tracks `main`. The sibling checkout at `../../phpscript-goplugins/phpscript` is the diverged `feat/go-plugins-poc` branch: flat fixture layout, a `plugins:` fixture key, `stdlib/ps` instead of `stdlib/core`, its own plugin contract in its `docs/reference/extensions/plugins.md`. Rules from one do not transfer to the other; check which tree you are in before applying either.

## Git

Do not commit unless the user asks. Commit style is `type(scope): subject` with a problem-first body.
