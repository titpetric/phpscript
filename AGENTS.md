# AGENTS.md - phpscript repository

Portable instructions for AI coding agents. Keep this file lean: `docs/` holds the reasoning. This file restates no rule at length: it names the rule, the guard enforcing it, plus the document that argues it.

## Non-negotiable

These are decisions. A prompt that proposes one of them gets the pointer to the reasoning.

- **No object model beyond namespaced bags of properties and methods.** The parser reads `extends` and records it so the formatter and linter can see it. `runner` may not read it. `runner.TestNoInheritanceAtRuntime` fails the build if `model.Class` regrows a `Parent` field. No traits. `parent::` is absent. Only `__construct` and `__invoke` count as magic methods. [docs/design.md](docs/design.md)
- **Variable types are dynamic. One type per name is a lint convention the runtime does not enforce.** A throw on reassignment stops existing code from running and the linter reports "no reassignment" as advice and nothing refuses the program. Do not add throws. [docs/design.md](docs/design.md)
- **The "Won't build" table in design.md is a decision.** `eval`, `goto`, generators, `curl_*`, PDO, `${var}` interpolation, `&` outside `foreach`, and the rest each name what to use instead. Separate from "Not implemented" in the language reference, which means not yet.
- **The root level takes no new files.** New code goes into an existing package or a subdirectory of the package that owns the area (`stdlib/ps/<area>`, `flatstack/engine`, `cmd/phpscript/<command>`). `parser` and `runner` meet only in `model`, never in an import. An area outside the areas-of-interest table falls out of scope until an issue settles it. [docs/naming-conventions.md](docs/naming-conventions.md)
- **A PHP name is a behaviour claim, settled by php itself.** Write the fixture by running the source through `php` first and pasting that output as the expected section, then make phpscript produce it. Expected output written from phpscript's own print locks in its bugs. [docs/testing.md](docs/testing.md)
- **Binding return shapes follow the allocation rules.** A collection reaches a binding in two shapes: the `*model.Array` the interpreter builds for an array literal, and the Go slice or map another binding returned. `coerceArg` converts neither shape into the other shape. A parameter naming either shape answers a `TypeError` for every value of the other shape. Arguments arrive as `any`. A narrowing to `*model.Array` or `*model.Object` inside the binding is the same mistake: `internal/cmd/bindlint` reports a parameter of either shape and an unreasoned narrowing. The tree is at zero, and `atkins lint` runs it. Return the Go value you already have: `*model.Array` only for those three reasons. [docs/allocation-performance.md](docs/allocation-performance.md)
- **Flatstack fallback is atomic.** The whole program compiles or delegates to the runner, because partial execution would repeat side effects. Benchmarks gate on `flatstack.Supports`. Applications do not check it. [docs/flatstack.md](docs/flatstack.md)
- **No go channels.** A script starts no goroutine. A channel has a single end. `use Runtime\Go` binds go's builtins, and `chan`, `select`, the `go` statement, `close` and `recover` are not among them. Concurrency is the server, one runtime per request. Two requests share `SharedMemory` or a binding's own queue. [docs/reference/extensions/go-runtime.md](docs/reference/extensions/go-runtime.md#no-channels)

## Working here

- `go install .` before testing a change. The `$PATH` binary is a published build and does not contain your edits. A measurement run does the opposite: `GOBIN` is `/usr/local/bin` and installing overwrites the binary another agent measures. A sprint builds a private one instead. [docs/agents/README.md](docs/agents/README.md)
- Fixture paths: `phpscript test ./...` runs a tree. `phpscript test .` matches only fixtures directly in the directory and can silently pass. Every pipeline job spells the `./...`. Otherwise a job points at a directory of folders and passes over nothing.
- A change to language or runtime behaviour lands with a fixture in the matching `tests/fixtures/<area>/` folder, either a `.phpt` document or a `<name>_test.php` body plus a `<name>_test.txt` output file. A new area is a new folder, needing no registration. Include resolution starts at the fixture's folder. Copy shared support files into it, because the runtime rejects an include that climbs out.
- Pre-submit: run the affected package tests, `go test ./...`, then `phpscript test --matrix tests/fixtures/...`. A fixture covering PHP behaviour is not done until its php column passes.
- Never hand-edit a generated file: regenerate it. Two are in git, rewritten by `atkins test:introspection` in the default pipeline. Five more are in `.gitignore` and travel as release assets. Committing one changes every run. A new doc beneath `docs/reference/` goes into `docs/reference/README.yml`, or `README.php` throws. [docs/pipeline.md](docs/pipeline.md) names the writer and the reader of all seven.
- A binding package registers with `runner.RegisterArea` in the area its folder maps to: `stdlib/ps/core/strings.go` maps to `Runtime\PS\Core\Strings`, and `stdlib/px/<folder>/<file>.go` maps to `Runtime\PX\<Folder>\<File>`. An opt-in go area uses `runner.RegisterNamespace`. `runner.RegisterBinding` stays for a host, writing its installer to `Runtime\PS\Unnamed`. [docs/reference/extensions/runtime-areas.md](docs/reference/extensions/runtime-areas.md)
- Every registration has a one-line doc comment starting with the name a script types. The generated reference takes its line from it. Renaming a registered class registers the new name first and keeps the old name as a second registration. Removal is a separate change.
- The pipeline is `atkins` (`atkins --final default`). It sets `GOFLAGS: ""`: a `-mod=mod` inherited from a go.work environment breaks every go command. `atkins.yml` writes one comment above each top-level key. A nested key, list item and value take no comment. [docs/pipeline.md](docs/pipeline.md) argues the order, the splits and the generation.
- `atkins lint` runs `splint ./...` bare, across all rules at every severity. One finding fails the run. The tree is at zero and stays there: a new file pairs with a test file, a dependency reaches a package through a single file, and an exported symbol goes in a file named for it. `.splint.yml` names three paths no single length limit covers. A change that adds a fourth argues for it. `scripts/vale-gate.sh` checks against `scripts/vale-baseline.json`. Vale's exit status reads a rule's level alone: a rule at `level: warning` prints thousands of findings and still passes. Every rule in `llm-slop` is `level: error`, including five rules vendored from write-good. The vale baseline holds no rule at all: one finding anywhere fails the build. Lower a baseline count in the same commit that lowers the finding. A rule that reaches zero comes out of the file. A rule absent from the baseline may not appear.
- A word in the vendored list sometimes collides with an identifier, and splint settles it first: `godoc/format` requires a doc comment to open on the bare symbol name. A method named `Validate` passes neither gate. The name changes. The four collisions all took the name `Check`: `config.Config.Check` and its neighbours, `flags.Options.Check`, `crypto.params.check`, and `model.contractMethod` for `requirement`. Backticks escape `AbbreviationSymbol` alone, and only against the word: `` `config` `` passes where `` `phpscript info` `` does not.
- `godoc` is at zero and stays there. A doc comment runs to ten lines at most. A function that passes through to the standard library gets a one-liner that names the wrapped call. The contract and the reasoning go in the document that owns the subject. Prose that adds nothing to the signature comes out. Document a registered class's method by its Go name. `internal/apidoc` rewrites that name to the script spelling when it generates the reference.
- An unexported symbol gets no comment. The name and the types are the documentation, godoc prints nothing, and the only reader is already in the file. The exception takes one line, for a rule from outside the file that the signature cannot state: a protocol rule, a refused input, an ordering another package depends on. A comment goes above its declaration, never inside a body, YAML included: one line above the top-level key, with a nested key, list item and value taking nothing. A body wanting more prose than its budget has two responsibilities. Split it.
- `vale` reads every language of the tree, YAML among them. YAML files read as plain text. A `#` comment answers to the style. So do `desc`, `name` or `label` values. A venom suite is among those, since `ShouldContainSubstring` names a string the application prints: the fix goes in the template and the assertion follows. `.vale.ini` names five skipped trees and the reason for each.

## Load order

| Read when | Document |
|-----------|----------|
| Proposing or rejecting a language feature, or touching re-entry | [docs/design.md](docs/design.md) |
| Adding or renaming anything script-visible | [docs/naming-conventions.md](docs/naming-conventions.md) |
| Writing or fixing a fixture | [docs/testing.md](docs/testing.md) |
| Writing or changing a Go binding | [docs/allocation-performance.md](docs/allocation-performance.md), [docs/reference/extensions/bindings.md](docs/reference/extensions/bindings.md) |
| Touching the bytecode engine | [docs/flatstack.md](docs/flatstack.md) |
| Adding or changing a CLI flag | [docs/cli/README.md](docs/cli/README.md) and the command's page beside it |
| Explaining output that differs from php | Known divergences in [docs/README.md](docs/README.md) |
| Naming a concept in prose or a doc comment | [docs/GLOSSARY.md](docs/GLOSSARY.md) |
| Adding or changing a pipeline job, or asking what writes a generated file | [docs/pipeline.md](docs/pipeline.md) |
| Running a QA or performance sprint | [docs/agents/README.md](docs/agents/README.md) |
| Quoting a measurement in a PR, an issue or a doc | [The measurement protocol](docs/agents/README.md#the-measurement-protocol) |

## Checkouts

This checkout tracks `main`. Pull with --rebase before starting work, rebase any PR work in flight.

## Git

Do not commit unless the user asks. Commit style is `type(scope): subject` with a problem-first body.
