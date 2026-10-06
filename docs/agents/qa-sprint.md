# QA sprint

Find and diagnose bugs, delete what nothing reaches, cut code without changing behaviour, and leave every finding as a fixture.

Read this when running a correctness pass over the tree. It runs under the contract in [README.md](README.md); that document is not restated here.

## Scope

Four deliverables, in the order they pay off:

- A diagnosed bug, with the fixture that pins it, and the fix.
- Code nothing reaches, deleted.
- Two implementations of one thing, reduced to one.
- A reproduction for a behaviour that is wrong but not yet fixed.

Out of scope: new features, and anything the contract puts out of scope. A finding that needs a feature becomes an issue from the feature-gap template, not a half fix - [../design.md](../design.md) decides whether it is a gap or a decision, and a row in its "Won't implement" table is a decision.

A finding the operator must decide is written up and left. It is not guessed at.

## Where bugs are

In priority order, each with the command that surfaces it.

**A matrix run whose three columns disagree.** The strongest signal in the repo: a fixture that passes on the interpreter and fails on flatstack is an engine divergence, and one that passes on both and fails on `php` is a compatibility defect.

```sh
phpscript test --matrix -v ./tests/...
```

A `SKIP` is a fixture that opted a runtime out, or a missing `php` binary. Any other non-pass fails the run.

**The known divergences, read as claims.** `../README.md` carries a long list of places phpscript differs from PHP, and nearly every entry names the fixture that pins it. Re-run those fixtures against `php` directly. An entry whose fixture no longer demonstrates what the prose says is either a fixed divergence nobody deleted the paragraph for, or a new one.

**The advisory findings.** `phpscript lint ./...` over the demo trees and the fixture corpus. Undefined names, unreachable code and unused variables. A finding over `demos/` is worth more than one over a fixture, because a demo is real code.

**The bindings the audit marks TODO.** `../allocation-performance.md` carries a per-binding audit whose legend distinguishes optimal, allocates-by-design, and a real improvement available. The TODO rows are a worklist that has already been triaged.

**A thin fixture area.** `../test-fixtures.md` is one table per area. An area with three fixtures covering a stdlib surface with thirty functions is where a defect is sitting unobserved.

**`tests/fixtures/github/`** is the log of what was reported, not a backlog: the three files there are closed, each naming the regression test that now guards it. It is worth reading for the shape of a good report, not for open work.

## Diagnosis

A diagnosis comes before a fix and has four parts:

- The smallest source that shows it. Not the program it was found in.
- What `php` prints for that same source. Run it; do not reason about it.
- The file and line that decides. Not the file the symptom appears in.
- Whether it is a bug or a documented divergence.

That last part is the one that gets skipped. `../README.md` carries divergences as deliberate claims. A fix that contradicts one of them is two changes: the behaviour and the paragraph that promised the old behaviour. Where the divergence is load-bearing, the fix is wrong and the finding is a documentation defect. Where it is stale, the paragraph goes in the same pull request. Either way the operator sees the claim named.

Three groups have no `php` to appeal to, and [../testing.md](../testing.md) lists them: host bindings, runtime introspection, and host request state. A diagnosis in one of those states what defines the expected output instead.

## The fixture

The fixture lands failing and the pull request makes it pass. One that passes before the fix proves nothing.

Where it goes:

| The behaviour is                                                                                    | Where the reproduction goes                                                                                          |
|-----------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| Reachable from a fixture                                                                            | A `.phpt` in the matching `tests/fixtures/<area>/`, or a `<name>_test.php` with its `<name>_test.txt` beside it      |
| Not reachable from a fixture, and not from route registration, server flags or host bindings either | `tests/github/issue_NNN_test.go` holding `Test_IssueNNN`, with a `.yml` in `tests/fixtures/github/` naming that test |
| Reported but not yet diagnosed                                                                      | `tests/fixtures/github/<n>.yml`, written against the behaviour that is wanted                                        |

The expected section is `php`'s output, pasted. [../testing.md](../testing.md) owns the format, the metadata fields, the two fixture forms and the oracle rule; none of that is repeated here.

A new area is a new folder and nothing registers it. The fixture's own folder is its include root, so a shared support file is copied rather than included from above.

## Dead code

`splint` reports uncovered symbols, `uncover` reads a coverage profile, and `scripts/splint-baseline.json` is the ratchet that decides whether a count may grow.

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  go test -count=1 -cover -coverpkg=./... -coverprofile=cover/qa.cov ./...
'
uncover cover/qa.cov
./scripts/splint-gate.sh
```

Four false positives, each real in this tree. A symbol reached only:

- from a benchmark, which `go test` without `-bench` never runs;
- from a `//go:build ignore` file, which `testdata/testserver.go` is;
- from a demo or a venom suite, which no Go test reads;
- by registered name, which every binding is. A binding is called by string from PHP and has no Go caller at all.

Two hard rules. A test is never dead code, which the contract states and which makes deleting one an edit under the test-immutability rule. And a registered name is removed in its own change: `../../AGENTS.md` requires a rename to register the new name and keep the old one as a second registration, so a removal that rides along with a rename is two changes in one.

When a count reaches zero in `scripts/splint-baseline.json`, the rule comes out of the file, which makes it blocking from then on. Lower a count in the same commit that lowered the finding.

## Simplification

What counts: one implementation where there were two. A helper with one caller, inlined. A branch no input reaches. An error path that cannot happen.

The gate is that behaviour is identical:

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  CGO_ENABLED=0 go build -o bin/phpscript .
  PATH="$PWD/bin:$PATH"
  command -v phpscript | grep -q "^$PWD/bin/" || exit 1
  go test ./...
  phpscript test --matrix ./tests/...
'
```

Both unchanged, and no benchmark moved outside benchstat's noise band. A simplification that changes a number is not a simplification.

Where the simplification *is* the optimisation, it belongs to [performance.md](performance.md) instead, because that pull request shape already requires the numbers that justify it.

## Finishing

One commit per finding, on the one draft branch the contract's "Where work lands" section defines. The body shape, the verdict command and the considered-and-rejected entry are in the contract.

Before opening: the affected package tests, then `go test ./...`, then `phpscript test --matrix tests/fixtures/...`, then `atkins --final default`.
