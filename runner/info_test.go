package runner_test

import (
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
)

// TestInfoSection covers when a registered section runs and where its rows
// print. It runs at the call and not at registration, so a section reports a
// count the process reached since; and sections print under their own heading
// in registration order, so a report reads the same way twice.
func TestInfoSection(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})

	calls := 0
	counter := runner.InfoSection(func(*runner.Runtime) []runner.InfoField {
		calls++
		return []runner.InfoField{{Name: "Calls", Value: strconv.Itoa(calls)}}
	})
	rt.RegisterInfo("First", counter)
	rt.RegisterInfo("Second", func(*runner.Runtime) []runner.InfoField {
		return []runner.InfoField{{Name: "Rows", Value: "1"}}
	})

	if calls != 0 {
		t.Fatalf("the section ran %d times at registration, want none", calls)
	}

	report := infoOutput(t, rt, &out)
	if calls != 1 {
		t.Errorf("the section ran %d times for one phpinfo(), want once", calls)
	}
	if !strings.Contains(report, "Calls => 1") {
		t.Errorf("the section's row is absent:\n%s", report)
	}
	first, second := strings.Index(report, "First"), strings.Index(report, "Second")
	if first < 0 || second < 0 {
		t.Fatalf("a heading is missing:\n%s", report)
	}
	if second < first {
		t.Errorf("the sections printed out of registration order:\n%s", report)
	}
}

// infoOutput runs phpinfo() on rt and answers what it printed.
func infoOutput(t *testing.T, rt *runner.Runtime, out *strings.Builder) string {
	t.Helper()
	if err := rt.PHPInfo(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestPHPInfoLeavesOutASectionWithNothingToSay(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	rt.RegisterInfo("Quiet", func(*runner.Runtime) []runner.InfoField { return nil })

	report := infoOutput(t, rt, &out)
	if strings.Contains(report, "Quiet") {
		t.Fatalf("a section reporting no rows printed its heading:\n%s", report)
	}
	// A runtime that loaded nothing has an empty cache, so the built-in
	// section is absent for the same reason.
	if strings.Contains(report, "OPcache") {
		t.Fatalf("an empty cache printed a section:\n%s", report)
	}
}

func TestPHPInfoReportsTheCompiledTree(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{
		Precompile: true,
		RootFS: fstest.MapFS{
			"lib.php": {Data: []byte(`<?php function helper($a) { return $a * 2; }`)},
		},
	})
	rt.SetIncludeCache(runner.NewIncludeCache())
	rt.SetExprCache(runner.NewExprCache())
	if _, err := rt.LoadFile("lib.php"); err != nil {
		t.Fatal(err)
	}

	report := infoOutput(t, rt, &out)
	if !strings.Contains(report, "\nOPcache\n") {
		t.Fatalf("no OPcache section:\n%s", report)
	}
	if !strings.Contains(report, "Cached Files => 1") {
		t.Fatalf("the cached file is not reported:\n%s", report)
	}
	// Nothing measured a size, so the section claims none.
	if strings.Contains(report, "Cached Size") {
		t.Fatalf("a size was reported with no precompile pass behind it:\n%s", report)
	}
}

func TestPHPInfoReportsAMeasuredSize(t *testing.T) {
	sources := fstest.MapFS{}
	for i := range 50 {
		sources[string(rune('a'+i%26))+strings.Repeat("x", i)+".php"] = &fstest.MapFile{
			Data: []byte(`<?php function f($a, $b) { return $a + $b; }`),
		}
	}
	includes := runner.NewIncludeCache()
	runner.Precompiler{Root: sources, Includes: includes, Exprs: runner.NewExprCache()}.Run()

	var out strings.Builder
	rt := runner.New(&out, runner.Options{Precompile: true, RootFS: sources})
	rt.SetIncludeCache(includes)

	report := infoOutput(t, rt, &out)
	if !strings.Contains(report, "Cached Size") {
		t.Fatalf("a measured size is not reported:\n%s", report)
	}
	if !strings.Contains(report, " MiB") {
		t.Fatalf("the size is not in MiB:\n%s", report)
	}
}
