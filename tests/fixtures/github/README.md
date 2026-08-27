# Issue reproductions

One file per GitHub issue, in the `.phpt` format under a `.yml` extension. The
fixture runner only discovers `.phpt`, so nothing here runs; the extension is
what keeps a file written against *expected* behaviour out of the passing
suite. Files stay after their issue closes, as the log of what was reported
and what now guards it.

While the issue is open, the file records the behaviour that is wanted.

When it closes, one of two things happens. If the fixture runner can reach the
behaviour, rename the file to `.phpt`, move it to the area it belongs to, and
close the issue with it: it joins the matrix and is a fixture like any other.

If it cannot, and route registration, server flags and host bindings cannot,
the regression test goes to [`tests/github`](../../github) as
`issue_NNN_test.go` holding `Test_IssueNNN`. The file here stays, with its
description rewritten to name that test. It is the evidence, not the check.
