# QA sprint

Find and diagnose bugs, delete what nothing reaches, cut code without changing behaviour, and leave every finding as a fixture.

Read this when running a correctness pass over the tree. The contract in [README.md](README.md) governs this work and is not restated here.

## Scope

Four deliverables, in the order to produce them:

- A diagnosed bug, with the fixture that pins it, plus the fix.
- Code nothing reaches, deleted.
- Two implementations of the same subject, reduced to a single implementation.
- A reproduction for a behaviour that is wrong but not yet fixed.

Out of scope: new features, and anything the contract puts out of scope. A finding that needs a feature becomes an issue from the feature-gap template, not a half fix - [../design.md](../design.md) decides whether the finding is a gap or a decision. A row in its "Won't build" table is a decision.

Write up a finding the operator must decide and leave it. Do not guess.

## Where bugs are

In priority order, each with the command that surfaces it.

**A matrix run whose three columns disagree.** The strongest signal in the repository. A fixture that passes on the interpreter and fails on flatstack is an engine divergence. A fixture passing on the interpreter and flatstack but failing on `php` is a compatibility defect.

```sh
phpscript test --matrix -v ./tests/...
```

A `SKIP` is a fixture that opted a runtime out, or a missing `php` binary. Any other non-pass fails the run.

**The known divergences, read as claims.** `../README.md` lists the places phpscript differs from PHP, and nearly every entry names the fixture that pins it. Re-run those fixtures against `php` directly. An entry whose fixture fails to show the documented divergence is either a stale paragraph for a fixed divergence, or a new divergence.

**The advisory findings.** `phpscript lint ./...` over the demo trees and the fixture corpus. Undefined names, unreachable code and unused variables. A finding over `demos/` matters more than a fixture finding: a demo is real code.

**The bindings the audit marks TODO.** `../allocation-performance.md` holds a per-binding audit whose legend distinguishes optimal, allocates-by-design, and real improvement available. The TODO rows are a worklist the audit already triaged.

**A thin fixture area.** `../test-fixtures.md` holds one table per area. An area with three fixtures covering a stdlib surface of thirty functions hides a defect.

**`tests/fixtures/github/`** records each report. It keeps no backlog. Three files there hold closed reports, each naming the regression test that guards it. Read it for the shape of a good report, not for open work.

## Diagnosis

A diagnosis comes before a fix and has four parts:

- The smallest source that shows the bug. Leave out the surrounding program.
- What `php` prints for that same source. Run it yourself.
- The file and line that decides. The symptom's file is elsewhere.
- Whether the behaviour is a bug or a documented divergence.

The last part gets skipped. `../README.md` states each divergence as a claim. A fix that contradicts one of them is two changes: the behaviour and the paragraph that promised the old behaviour. Where something depends on the divergence, the fix is wrong and the finding is a documentation defect. Where the claim is stale, the paragraph goes in the same pull request. Either way the operator sees the claim named.

Three groups have no `php` to appeal to, and [../testing.md](../testing.md) lists them: host bindings, runtime introspection, and host request state. A diagnosis in one of those states what defines the expected output instead.

## The fixture

The fixture lands failing and the pull request makes it pass. One that passes before the fix proves nothing.

Where it goes:

| The behaviour is                                                                                    | Where the reproduction goes                                                                                          |
|-----------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| Reachable from a fixture                                                                            | A `.phpt` in the matching `tests/fixtures/<area>/`, or a `<name>_test.php` with its `<name>_test.txt` beside it      |
| Not reachable from a fixture, and not from route registration, server flags or host bindings either | `tests/github/issue_NNN_test.go` holding `Test_IssueNNN`, with a `.yml` in `tests/fixtures/github/` naming that test |
| Reported but not yet diagnosed                                                                      | `tests/fixtures/github/<n>.yml`, written against the expected behaviour                                              |

The expected section is `php`'s output, pasted. [../testing.md](../testing.md) owns the format, the metadata fields, the two fixture forms and the oracle rule. This document repeats none of it.

A new area is a new folder. The runtime finds a new area without a registration. The fixture folder is the include root. Copy a shared support file into that folder: phpscript rejects an include that climbs out.

## Dead code

`splint` reports uncovered symbols and `uncover` reads a coverage profile. `splint` runs bare. One finding fails the run. A sprint reads its output as a list of work.

```sh
flock -w 3600 /tmp/phpscript-measure.lock bash -euc '
  go test -count=1 -cover -coverpkg=./... -coverprofile=cover/qa.cov ./...
'
uncover cover/qa.cov
splint ./...
```

Four false positives, each real in this tree. A symbol reached only:

- from a benchmark, which `go test` without `-bench` never runs,
- from a `//go:build ignore` file, which `testdata/goserver/main.go` is,
- from a demo or a venom suite, which no Go test reads,
- by registered name: every binding registers this way. A PHP source line calls a binding by its string name. It has no Go caller at all.

Two hard rules. A test is never dead code. The contract states this. It makes deleting a test an edit governed by the test-immutability rule. A registered name comes out in a separate change: `../../AGENTS.md` requires a rename to register the new name and keep the old name as a second registration. A removal in the same commit as a rename is two changes in one.

Every rule blocks. A finding is work. Do not turn it into a number. Three paths in `.splint.yml` are exempt from the length limit. That list is complete. A change that adds a fourth argues for it.

## Simplification

What counts: two implementations of the same subject, reduced to a single implementation. A helper with one caller, inlined. A branch no input reaches. An error path that cannot happen.

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

Both unchanged. No benchmark moved outside benchstat's noise band. A simplification that changes a number is not a simplification.

Where the simplification *is* the optimisation, it belongs to [performance.md](performance.md) instead, because that document already requires the numbers that justify it.

## Finishing

One commit per finding, on the draft branch the contract's "Where work lands" section defines. The body shape, the verdict command and the considered-and-rejected entry are in the contract.

Before opening, run the affected package tests, `go test ./...`, `phpscript test --matrix tests/fixtures/...`, and `atkins --final default`.
