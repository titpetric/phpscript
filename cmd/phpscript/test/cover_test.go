package test_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/cmd/phpscript/test"
)

// coverProfileLine finds the profile entry for file starting at line.
func coverProfileLine(t *testing.T, profile, file string, line int) string {
	t.Helper()
	prefix := fmt.Sprintf("%s:%d.", file, line)
	for _, entry := range strings.Split(profile, "\n") {
		if strings.HasPrefix(entry, prefix) {
			return entry
		}
	}
	t.Fatalf("profile has no entry for %s:%d:\n%s", file, line, profile)
	return ""
}

// TestRunCommandCover covers --cover, --coverfile and --split end to end: the
// merged profile is the go cover format under mode count, an included file
// appears with its unexecuted statements at zero, and the split profile lands
// next to the fixture.
func TestRunCommandCover(t *testing.T) {
	tmp := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		p := filepath.Join(tmp, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("suite/lib/functions.php", `<?php

function covered($n)
{
	return $n;
}

function uncovered()
{
	return "never";
}
`)
	write("suite/covered.phpt", `name: coverage fixture
description: exercises entrypoint and include coverage
---
<?php

include "lib/functions.php";

for ($i = 0; $i < 3; $i++) {
	echo covered($i);
}
---
012
`)

	t.Chdir(tmp)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	errRun := test.Run(context.Background(), []string{"suite"}, test.Options{Cover: test.CoverLine, Split: true})
	w.Close()
	os.Stdout = old
	var stdout bytes.Buffer
	_, _ = stdout.ReadFrom(r)
	if errRun != nil {
		t.Fatalf("run with --cover --split: %v\n%s", errRun, stdout.String())
	}
	// Without -v the folder summary is the whole answer on stdout: the run
	// reports what it measured per folder rather than per fixture.
	for _, want := range []string{"| Path ", "| Files ", "| Lines ", "| suite "} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout is missing %q from the folder summary:\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "| Test |") {
		t.Errorf("stdout carries a fixture table without -v:\n%s", stdout.String())
	}

	data, err := os.ReadFile("phpscript.cov")
	if err != nil {
		t.Fatalf("read merged profile: %v", err)
	}
	profile := string(data)
	if !strings.HasPrefix(profile, "mode: count\n") {
		t.Fatalf("profile does not open with mode count:\n%s", profile)
	}

	// The echo line of the .phpt runs once per loop iteration; lines count from
	// the start of the PHP section, and the columns span the statement text.
	if got := coverProfileLine(t, profile, "suite/covered.phpt", 6); got != "suite/covered.phpt:6.2,6.19 1 3" {
		t.Errorf("entrypoint echo entry = %q", got)
	}
	// The include registers whole: the called function's body counts, and the
	// never-called one is present at zero.
	if got := coverProfileLine(t, profile, "suite/lib/functions.php", 5); !strings.HasSuffix(got, " 1 3") {
		t.Errorf("covered() body entry = %q, want count 3", got)
	}
	if got := coverProfileLine(t, profile, "suite/lib/functions.php", 10); !strings.HasSuffix(got, " 1 0") {
		t.Errorf("uncovered() body entry = %q, want count 0", got)
	}

	split, err := os.ReadFile("suite/covered.cov")
	if err != nil {
		t.Fatalf("read split profile: %v", err)
	}
	if !strings.HasPrefix(string(split), "mode: count\n") || !strings.Contains(string(split), "suite/lib/functions.php:") {
		t.Errorf("split profile is missing its content:\n%s", split)
	}
}

// TestRunCommandCoverConflicts holds the flag contract this command owns:
// coverage counts one run per fixture, so the benchmark loops are refused, and
// a report mode owns stdout, so --json is refused with it. The mode vocabulary
// itself belongs to internal/flags, which every command shares.
func TestRunCommandCoverConflicts(t *testing.T) {
	if err := test.Run(context.Background(), nil, test.Options{Cover: test.CoverLine, Count: 5}); err == nil || !strings.Contains(err.Error(), "cover") {
		t.Errorf("cover with count: err = %v, want a cover conflict", err)
	}
	if err := test.Run(context.Background(), nil, test.Options{Cover: test.CoverLine, Time: time.Second}); err == nil || !strings.Contains(err.Error(), "cover") {
		t.Errorf("cover with time: err = %v, want a cover conflict", err)
	}
	if err := test.Run(context.Background(), nil, test.Options{Cover: test.CoverFunc, JSON: true}); err == nil || !strings.Contains(err.Error(), "json") {
		t.Errorf("cover=func with json: err = %v, want a json conflict", err)
	}
}

// coverReportSuite writes the fixture tree the report-mode tests share: an
// include with a called and an uncalled function, a class with a called and an
// uncalled method, and a prelude at the invocation root, which the include
// union resolves outside the fixture's directory.
func coverReportSuite(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		p := filepath.Join(tmp, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("prelude.php", `<?php

function prelude_helper()
{
	return 42;
}

$prelude_value = prelude_helper();
`)
	write("suite/lib/functions.php", `<?php

function covered($n)
{
	return $n;
}

function uncovered()
{
	return "never";
}
`)
	write("suite/lib/greeter.php", `<?php

class Greeter
{
	public function greet($n)
	{
		return "hi" . $n;
	}

	public function unused()
	{
		return "no";
	}
}
`)
	write("suite/lib/contract.php", `<?php

interface Contract
{
	public function greet($n);
}
`)
	write("suite/covered.phpt", `name: coverage report fixture
description: exercises functions, methods and the include union
---
<?php

include "lib/contract.php";
include "lib/functions.php";
include "lib/greeter.php";

$g = new Greeter();
echo covered(1), $g->greet(2), prelude_helper();
---
1hi242
`)
	t.Chdir(tmp)
}

// runCoverReport runs the suite in a report mode and returns stdout.
func runCoverReport(t *testing.T, opts test.Options) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	errRun := test.Run(context.Background(), []string{"suite"}, opts)
	w.Close()
	os.Stdout = old
	var stdout bytes.Buffer
	_, _ = stdout.ReadFrom(r)
	if errRun != nil {
		t.Fatalf("run with --cover=%s: %v\n%s", opts.Cover, errRun, stdout.String())
	}
	return stdout.String()
}

// reportLine finds the report row starting with prefix.
func reportLine(t *testing.T, report, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("report has no row starting %q:\n%s", prefix, report)
	return ""
}

// TestRunCommandCoverFunc covers --cover=func: every declaration reports under
// its file and start line, methods under Class::name, top-level code as {main},
// the prelude under the path the include union served it from, and neither the
// fixture tables nor any .phpt row reaches stdout.
func TestRunCommandCoverFunc(t *testing.T) {
	coverReportSuite(t)
	report := runCoverReport(t, test.Options{Cover: test.CoverFunc, Include: "prelude.php"})

	for prefix, want := range map[string]string{
		"suite/lib/functions.php:3:": "covered 100.0%",
		"suite/lib/functions.php:8:": "uncovered 0.0%",
		"suite/lib/greeter.php:5:":   "Greeter::greet 100.0%",
		"suite/lib/greeter.php:10:":  "Greeter::unused 0.0%",
		"prelude.php:3:":             "prelude_helper 100.0%",
		"prelude.php:1:":             "{main} 100.0%",
		// The interface file holds nothing runnable, so its coverage is the
		// adjusted 0/0: nothing left uncovered.
		"suite/lib/contract.php:1:": "{main} 100.0%",
	} {
		got := reportLine(t, report, prefix)
		if cells := strings.Fields(got); strings.Join(cells[1:], " ") != want {
			t.Errorf("row %q = %q, want columns %q", prefix, got, want)
		}
	}
	if !strings.Contains(report, "total:") {
		t.Errorf("report is missing the total row:\n%s", report)
	}
	if strings.Contains(report, ".phpt") {
		t.Errorf("report leaks fixture rows or tables:\n%s", report)
	}
	if strings.Contains(report, "suite/prelude.php") {
		t.Errorf("report names the prelude below the fixture directory:\n%s", report)
	}

	// The merged profile names the prelude as the union resolved it: at the
	// invocation root, not below the fixture's directory.
	data, err := os.ReadFile("phpscript.cov")
	if err != nil {
		t.Fatalf("read merged profile: %v", err)
	}
	if !strings.Contains(string(data), "\nprelude.php:") || strings.Contains(string(data), "suite/prelude.php:") {
		t.Errorf("profile does not name the prelude at the invocation root:\n%s", data)
	}
}

// TestRunCommandCoverFile covers --cover=file: one row per source file with a
// statement-weighted percentage, and no fixture rows.
func TestRunCommandCoverFile(t *testing.T) {
	coverReportSuite(t)
	report := runCoverReport(t, test.Options{Cover: test.CoverFile, Include: "prelude.php"})

	for prefix, want := range map[string]string{
		"suite/lib/functions.php:1:": "functions.php 50.0%",
		"suite/lib/greeter.php:1:":   "greeter.php 50.0%",
		"suite/lib/contract.php:1:":  "contract.php 100.0%",
		"prelude.php:1:":             "prelude.php 100.0%",
	} {
		got := reportLine(t, report, prefix)
		if cells := strings.Fields(got); strings.Join(cells[1:], " ") != want {
			t.Errorf("row %q = %q, want columns %q", prefix, got, want)
		}
	}
	if !strings.Contains(report, "total:") || strings.Contains(report, ".phpt") {
		t.Errorf("report total/fixture contract broken:\n%s", report)
	}
}

// TestRunCommandFolderSummary covers what a run without -v prints: one row per
// folder resolved from the arguments, carrying the two coverage counts, and no
// fixture table at all.
func TestRunCommandFolderSummary(t *testing.T) {
	coverReportSuite(t)
	report := runCoverReport(t, test.Options{Cover: test.CoverLine, Include: "prelude.php"})

	// Four files are loaded and three hold runnable statements; the interface
	// counts as reached because it has nothing left to reach.
	row := reportLine(t, report, "| suite ")
	for _, want := range []string{"4/4 (100%)", "4/6 (67%)"} {
		if !strings.Contains(row, want) {
			t.Errorf("folder row = %q, want %q in it", row, want)
		}
	}
	if strings.Contains(report, "| Test |") || strings.Contains(report, ".phpt") {
		t.Errorf("folder summary leaks fixture rows or tables:\n%s", report)
	}
	// The one-line percentage measures the written profile, which counts the
	// .phpt entrypoints; the table does not. Printing both invites a comparison
	// between two numbers answering different questions, so it waits for -v.
	if strings.Contains(report, "% of statements") {
		t.Errorf("folder summary carries the profile percentage without -v:\n%s", report)
	}
	if !strings.Contains(report, "Test summary: 1 passed, 0 failed") {
		t.Errorf("folder summary is missing the run total:\n%s", report)
	}
}

// TestRunCommandFolderSummaryVerbose covers the other half of the same switch:
// -v restores the fixture tables, gives each one a coverage column, and drops
// to a per-file report under every folder that loaded a PHP file.
func TestRunCommandFolderSummaryVerbose(t *testing.T) {
	coverReportSuite(t)
	report := runCoverReport(t, test.Options{Cover: test.CoverLine, Include: "prelude.php", Verbose: true})

	if !strings.Contains(report, "| Test |") || !strings.Contains(report, "Coverage") {
		t.Errorf("verbose run is missing the fixture table or its coverage column:\n%s", report)
	}
	if strings.Contains(report, "| Path ") {
		t.Errorf("verbose run carries the folder summary as well as the tables:\n%s", report)
	}
	if !strings.Contains(report, "## coverage: suite") {
		t.Errorf("verbose run is missing the per-file coverage section:\n%s", report)
	}
	for _, want := range []string{"suite/lib/functions.php", "suite/lib/greeter.php", "prelude.php"} {
		if !strings.Contains(report, want) {
			t.Errorf("per-file coverage is missing %q:\n%s", want, report)
		}
	}
	if !strings.Contains(report, "% of statements") {
		t.Errorf("verbose run is missing the profile percentage:\n%s", report)
	}
}

// TestRunCommandFolderSummaryWithoutCover covers the summary a run prints when
// it measured no coverage: the same one row per folder, without the two columns
// there is nothing to fill.
func TestRunCommandFolderSummaryWithoutCover(t *testing.T) {
	coverReportSuite(t)
	report := runCoverReport(t, test.Options{Include: "prelude.php"})

	if !strings.Contains(report, "| Path ") || !strings.Contains(report, "| suite ") {
		t.Errorf("run is missing the folder summary:\n%s", report)
	}
	if strings.Contains(report, "| Files ") || strings.Contains(report, "| Lines ") {
		t.Errorf("run carries coverage columns it did not measure:\n%s", report)
	}
	if strings.Contains(report, "| Test |") {
		t.Errorf("run carries a fixture table without -v:\n%s", report)
	}
}
