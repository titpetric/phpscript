# Issue reproductions

One file per GitHub issue, in the `.phpt` format under a `.yml` extension. The
fixture runner only discovers `.phpt`, and the extension keeps a file written
against *expected* behaviour out of the passing suite. Files stay after their issue closes, as the record of each report
and what now guards it.

While the issue is open, the file records the expected behaviour.

When it closes, one of two things happens. If the fixture runner can reach the
behaviour, rename the file to `.phpt`, move it to the area it belongs to, and
close the issue with it: it joins the matrix and is a fixture like any other.

If it cannot, and route registration, server flags and host bindings cannot,
the regression test goes to [`tests/github`](../../github) as
`issue_NNN_test.go` holding `Test_IssueNNN`. The file here stays, with its
description rewritten to name that test. The fixture is the evidence; the Go test is the check.
