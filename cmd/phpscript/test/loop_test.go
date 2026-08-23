package test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/titpetric/phpscript/tests"
)

func loopFixture(t *testing.T) *tests.Fixture {
	t.Helper()
	src := "name: loop\ndescription: loop harness fixture\n---\n<?php echo \"ok\";\n---\nok"
	fx, err := tests.ParseFixture([]byte(src), "loop.phpt")
	if err != nil {
		t.Fatal(err)
	}
	return fx
}

func TestRunFixtureLoopCount(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), tests.RunnerRuntime, Options{Count: 5})
	if fr.Runs != 5 {
		t.Fatalf("Runs = %d, want 5", fr.Runs)
	}
	if !fr.Result.Passed {
		t.Fatalf("fixture failed: %s", fr.Result.FailureReason)
	}
}

func TestRunFixtureLoopTime(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), tests.RunnerRuntime, Options{Time: 50 * time.Millisecond})
	if fr.Runs < 2 {
		t.Fatalf("Runs = %d, want at least 2 within 50ms", fr.Runs)
	}
	if fr.Total < 50*time.Millisecond {
		t.Fatalf("Total = %v, want >= 50ms", fr.Total)
	}
}

func TestRunFixtureLoopSingleByDefault(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), tests.RunnerRuntime, Options{})
	if fr.Runs != 1 {
		t.Fatalf("Runs = %d, want 1", fr.Runs)
	}
}

func TestRunFixtureLoopProfile(t *testing.T) {
	fr := runFixtureLoop(context.Background(), loopFixture(t), tests.RunnerRuntime, Options{Count: 3, Profile: true})
	if fr.AllocsPerOp == 0 || fr.BytesPerOp == 0 {
		t.Fatalf("profile counters empty: allocs=%d bytes=%d", fr.AllocsPerOp, fr.BytesPerOp)
	}
}

func TestRunFixtureSamplesCountAndTime(t *testing.T) {
	const (
		count     = 3
		benchTime = 10 * time.Millisecond
	)
	runs := runFixtureSamples(context.Background(), loopFixture(t), tests.RunnerRuntime, Options{
		Count: count,
		Time:  benchTime,
	})
	if len(runs) != count {
		t.Fatalf("sample count = %d, want %d", len(runs), count)
	}
	for i, run := range runs {
		if run.Total < benchTime {
			t.Errorf("sample %d duration = %v, want at least %v", i, run.Total, benchTime)
		}
		if run.Runs < 1 {
			t.Errorf("sample %d operation count = %d, want at least 1", i, run.Runs)
		}
	}
}

func TestTerminalTableColumnsAndSpacing(t *testing.T) {
	fr := &fixtureRun{
		Result:      &tests.TestResult{Passed: true},
		DisplayPath: "arrays/a.phpt",
		Label:       "a.phpt",
		fixtureMetrics: fixtureMetrics{
			Runs:        2,
			Total:       3 * time.Millisecond,
			AllocsPerOp: 7,
			BytesPerOp:  11,
			GCRuns:      2,
		},
	}
	cases := []struct {
		opts   Options
		header string
	}{
		{Options{}, "| Test | arrays | Duration (ms) | GC Runs |"},
		{Options{Count: 2}, "| Test | arrays | Duration (ms) | Count | GC Runs |"},
		{Options{Count: 2, Profile: true}, "| Test | arrays | Duration (ms) | Count | Allocs/op | Bytes/op | GC Runs |"},
		{Options{Profile: true}, "| Test | arrays | Duration (ms) | Allocs/op | Bytes/op | GC Runs |"},
		{Options{Time: time.Millisecond}, "| Test | arrays | Duration (ms) | Count | P50 (µs) | P95 (µs) | P99 (µs) | GC Runs |"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		table := newTerminalTable(&buf, c.opts)
		table.writeGroup("arrays", []string{"a-much-longer-name.phpt"})
		table.writeResult(fr)
		table.closeGroup(groupTotals{Dir: "arrays", Passed: 1, Total: 1})
		output := ansi.Strip(buf.String())
		// The trailing lines are the folder subtotal and its blank line, which
		// are prose rather than table rows.
		lines := strings.Split(strings.TrimSpace(output), "\n")
		lines = lines[:len(lines)-1]
		header := strings.Join(strings.Fields(strings.ReplaceAll(lines[1], "│", "|")), " ")
		if header != c.header {
			t.Errorf("opts %+v: header = %q, want %q", c.opts, header, c.header)
		}
		for i, line := range lines {
			if ansi.StringWidth(line) != ansi.StringWidth(lines[0]) {
				t.Errorf("opts %+v: line %d has width %d, want %d:\n%s", c.opts, i, ansi.StringWidth(line), ansi.StringWidth(lines[0]), output)
			}
		}
		if !strings.Contains(buf.String(), colorSeparator+boxTopLeft) ||
			!strings.Contains(buf.String(), colorHeader+"Test") ||
			!strings.Contains(buf.String(), colorGreen+"PASS") {
			t.Errorf("opts %+v: table is missing expected ANSI colors: %q", c.opts, buf.String())
		}
		if table.loop && table.widths[3] != 7 {
			t.Errorf("opts %+v: Count width = %d, want 7", c.opts, table.widths[3])
		}
	}
}

func TestResultTableFallsBackToMarkdown(t *testing.T) {
	var buf bytes.Buffer
	table := newResultTable(&buf, Options{}, true)
	table.writeGroup("arrays", []string{"a.phpt"})
	table.writeResult(&fixtureRun{
		Result:         &tests.TestResult{Passed: true},
		DisplayPath:    "arrays/a.phpt",
		Label:          "a.phpt",
		fixtureMetrics: fixtureMetrics{Total: time.Millisecond},
	})
	table.closeGroup(groupTotals{Dir: "arrays", Passed: 1, Total: 1, Duration: time.Millisecond})
	table.writeSummary(1, 0, 1, time.Millisecond)

	want := "## arrays\n\n" +
		"| Test | arrays | Duration (ms) | GC Runs         |\n" +
		"| ---- | ------ | ------------- | --------------- |\n" +
		"| PASS | a.phpt | 1             | 0 (0.00%)       |\n" +
		"\n" +
		"## Summary\n\n" +
		"| Area      | Fixtures | Passed | Failed |\n" +
		"| --------- | -------- | ------ | ------ |\n" +
		"| arrays    | 1        | 1      | 0      |\n" +
		"| **Total** | 1        | 1      | 0      |\n" +
		"\n"
	if got := buf.String(); got != want {
		t.Fatalf("markdown table =\n%s\nwant:\n%s", got, want)
	}
}

// TestMarkdownTableGroupsAndSummary asserts the property that makes the
// generated report trustworthy: the Total row is the sum of the folder rows.
func TestMarkdownTableGroupsAndSummary(t *testing.T) {
	var buf bytes.Buffer
	table := newMarkdownTable(&buf, Options{})

	for _, group := range []struct {
		dir            string
		passed, failed int
	}{{"arrays", 2, 0}, {"strings", 1, 1}} {
		table.writeGroup(group.dir, []string{"a.phpt"})
		for i := 0; i < group.passed+group.failed; i++ {
			table.writeResult(&fixtureRun{
				Result: &tests.TestResult{Passed: i < group.passed},
				Label:  "a.phpt",
			})
		}
		table.closeGroup(groupTotals{
			Dir:    group.dir,
			Passed: group.passed,
			Failed: group.failed,
			Total:  group.passed + group.failed,
		})
	}
	table.writeSummary(3, 1, 4, time.Millisecond)

	got := buf.String()
	for _, want := range []string{"## arrays", "## strings", "## Summary", "| **Total** | 4        | 3      | 1      |"} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}

	var sumTotal, sumPassed, sumFailed int
	for _, totals := range table.totals {
		sumTotal += totals.Total
		sumPassed += totals.Passed
		sumFailed += totals.Failed
	}
	if sumTotal != 4 || sumPassed != 3 || sumFailed != 1 {
		t.Fatalf("folder totals sum to %d/%d/%d, want 4/3/1", sumTotal, sumPassed, sumFailed)
	}
}

func TestCountColumnExpandsForConfiguredCount(t *testing.T) {
	table := newTerminalTable(&bytes.Buffer{}, Options{Count: 100000000})
	table.sizeColumns("arrays", nil)
	if got := table.widths[3]; got != 9 {
		t.Fatalf("Count width = %d, want 9", got)
	}
}

func TestTerminalSummaryUsesWorktreeStyle(t *testing.T) {
	var buf bytes.Buffer
	table := newTerminalTable(&buf, Options{})
	table.writeSummary(2, 1, 3, 4*time.Millisecond)
	want := colorHeader + "Test summary: 2 passed, 1 failed out of 3 fixtures (4ms)" + colorReset + "\n"
	if got := buf.String(); got != want {
		t.Fatalf("terminal summary = %q, want %q", got, want)
	}
}

// TestTerminalGroupSubtotal covers the line closeGroup adds under each folder.
func TestTerminalGroupSubtotal(t *testing.T) {
	var buf bytes.Buffer
	table := newTerminalTable(&buf, Options{})
	table.sizeColumns("arrays", []string{"a.phpt"})
	table.closeGroup(groupTotals{Dir: "arrays", Passed: 4, Failed: 1, Total: 5, Duration: 6 * time.Millisecond})
	if want := "arrays: 4 passed, 1 failed out of 5 fixtures (6ms)"; !strings.Contains(ansi.Strip(buf.String()), want) {
		t.Fatalf("group subtotal = %q, want it to contain %q", buf.String(), want)
	}
}

func TestFormatGCRuns(t *testing.T) {
	if got, want := formatGCRuns(2, 8), "2 (25.00%)"; got != want {
		t.Fatalf("formatGCRuns(2, 8) = %q, want %q", got, want)
	}
}

func TestPercentileNs(t *testing.T) {
	samples := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if got, want := percentileNs(samples, 50), int64(50); got != want {
		t.Fatalf("p50 = %d, want %d", got, want)
	}
	if got, want := percentileNs(samples, 95), int64(100); got != want {
		t.Fatalf("p95 = %d, want %d", got, want)
	}
	if got := percentileNs(nil, 50); got != 0 {
		t.Fatalf("empty = %d, want 0", got)
	}
}
