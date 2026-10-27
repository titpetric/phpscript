package test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/internal/table"
	"github.com/titpetric/phpscript/tests"
)

func TestTerminalTableColumnsAndSpacing(t *testing.T) {
	t.Parallel()
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
		tbl := newTerminalTable(&buf, c.opts)
		tbl.writeGroup("arrays", []string{"a-much-longer-name.phpt"})
		tbl.writeResult(fr)
		tbl.closeGroup(groupTotals{Dir: "arrays", Passed: 1, Total: 1})
		output := table.Strip(buf.String())
		// The trailing lines are the folder subtotal and its blank line, which
		// are prose and not table rows.
		lines := strings.Split(strings.TrimSpace(output), "\n")
		lines = lines[:len(lines)-1]
		header := strings.Join(strings.Fields(strings.ReplaceAll(lines[1], "│", "|")), " ")
		if header != c.header {
			t.Errorf("opts %+v: header = %q, want %q", c.opts, header, c.header)
		}
		for i, line := range lines {
			if table.Width(line) != table.Width(lines[0]) {
				t.Errorf("opts %+v: line %d has width %d, want %d:\n%s", c.opts, i, table.Width(line), table.Width(lines[0]), output)
			}
		}
		if !strings.Contains(buf.String(), table.ColorSeparator+table.BoxTopLeft) ||
			!strings.Contains(buf.String(), table.ColorHeader+"Test") ||
			!strings.Contains(buf.String(), table.ColorGreen+"PASS") {
			t.Errorf("opts %+v: table is missing expected ANSI colors: %q", c.opts, buf.String())
		}
		if tbl.loop && tbl.widths[3] != 7 {
			t.Errorf("opts %+v: Count width = %d, want 7", c.opts, tbl.widths[3])
		}
	}
}

func TestResultTableFallsBackToMarkdown(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tbl := newResultTable(&buf, Options{}, true)
	tbl.writeGroup("arrays", []string{"a.phpt"})
	tbl.writeResult(&fixtureRun{
		Result:         &tests.TestResult{Passed: true},
		DisplayPath:    "arrays/a.phpt",
		Label:          "a.phpt",
		fixtureMetrics: fixtureMetrics{Total: time.Millisecond},
	})
	tbl.closeGroup(groupTotals{Dir: "arrays", Passed: 1, Total: 1, Duration: time.Millisecond})
	tbl.writeSummary(1, 0, 1, time.Millisecond)

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
	t.Parallel()
	var buf bytes.Buffer
	tbl := newMarkdownTable(&buf, Options{})

	for _, group := range []struct {
		dir            string
		passed, failed int
	}{{"arrays", 2, 0}, {"strings", 1, 1}} {
		tbl.writeGroup(group.dir, []string{"a.phpt"})
		for i := 0; i < group.passed+group.failed; i++ {
			tbl.writeResult(&fixtureRun{
				Result: &tests.TestResult{Passed: i < group.passed},
				Label:  "a.phpt",
			})
		}
		tbl.closeGroup(groupTotals{
			Dir:    group.dir,
			Passed: group.passed,
			Failed: group.failed,
			Total:  group.passed + group.failed,
		})
	}
	tbl.writeSummary(3, 1, 4, time.Millisecond)

	got := buf.String()
	for _, want := range []string{"## arrays", "## strings", "## Summary", "| **Total** | 4        | 3      | 1      |"} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}

	var sumTotal, sumPassed, sumFailed int
	for _, totals := range tbl.totals {
		sumTotal += totals.Total
		sumPassed += totals.Passed
		sumFailed += totals.Failed
	}
	if sumTotal != 4 || sumPassed != 3 || sumFailed != 1 {
		t.Fatalf("folder totals sum to %d/%d/%d, want 4/3/1", sumTotal, sumPassed, sumFailed)
	}
}

func TestCountColumnExpandsForConfiguredCount(t *testing.T) {
	t.Parallel()
	tbl := newTerminalTable(&bytes.Buffer{}, Options{Count: 100000000})
	tbl.sizeColumns("arrays", nil)
	if got := tbl.widths[3]; got != 9 {
		t.Fatalf("Count width = %d, want 9", got)
	}
}

func TestTerminalSummaryUsesWorktreeStyle(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tbl := newTerminalTable(&buf, Options{})
	tbl.writeSummary(2, 1, 3, 4*time.Millisecond)
	want := table.ColorHeader + "Test summary: 2 passed, 1 failed out of 3 fixtures (4ms)" + table.ColorReset + "\n"
	if got := buf.String(); got != want {
		t.Fatalf("terminal summary = %q, want %q", got, want)
	}
}

// TestTerminalGroupSubtotal covers the line closeGroup adds under each folder.
func TestTerminalGroupSubtotal(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tbl := newTerminalTable(&buf, Options{})
	tbl.sizeColumns("arrays", []string{"a.phpt"})
	tbl.closeGroup(groupTotals{Dir: "arrays", Passed: 4, Failed: 1, Total: 5, Duration: 6 * time.Millisecond})
	if want := "arrays: 4 passed, 1 failed out of 5 fixtures (6ms)"; !strings.Contains(table.Strip(buf.String()), want) {
		t.Fatalf("group subtotal = %q, want it to contain %q", buf.String(), want)
	}
}

func TestFormatGCRuns(t *testing.T) {
	t.Parallel()
	if got, want := formatGCRuns(2, 8), "2 (25.00%)"; got != want {
		t.Fatalf("formatGCRuns(2, 8) = %q, want %q", got, want)
	}
}

// TestFormatDuration covers the one spelling every duration column prints in:
// whole milliseconds, because a reader compares a table of fixture times
// column by column, and a mixed unit would force a read per cell.
func TestFormatDuration(t *testing.T) {
	t.Parallel()
	for duration, want := range map[time.Duration]string{
		0:                       "0",
		999 * time.Microsecond:  "0",
		time.Millisecond:        "1",
		1500 * time.Microsecond: "1",
		6 * time.Millisecond:    "6",
		2 * time.Second:         "2000",
	} {
		if got := formatDuration(duration); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", duration, got, want)
		}
	}
}

// TestEngineTime reads one runner's share out of the per-engine split. A
// runner absent from the split has no figure, the state a matrix run leaves
// behind where every fixture in the folder skips an engine.
func TestEngineTime(t *testing.T) {
	t.Parallel()
	engines := []engineDuration{
		{Runner: tests.RunnerFlatstack, Duration: time.Millisecond},
		{Runner: tests.RunnerPHP, Duration: 3 * time.Millisecond},
	}
	if got := engineTime(tests.RunnerPHP, engines); got != 3*time.Millisecond {
		t.Errorf("engineTime(php) = %v, want 3ms", got)
	}
	if got := engineTime(tests.RunnerRuntime, engines); got != 0 {
		t.Errorf("engineTime of a runner the split omits = %v, want 0", got)
	}
	if got := engineTime(tests.RunnerPHP, nil); got != 0 {
		t.Errorf("engineTime over no split = %v, want 0", got)
	}
}

// TestMarkdownCell covers the escaping a fixture's failure reason needs. A
// pipe in the text would close the cell early and shift every column after it,
// making the generated report read as a different table.
func TestMarkdownCell(t *testing.T) {
	t.Parallel()
	if got, want := markdownCell("a | b"), `a \| b`; got != want {
		t.Errorf("markdownCell = %q, want %q", got, want)
	}
	if got, want := markdownCell("plain"), "plain"; got != want {
		t.Errorf("markdownCell = %q, want %q", got, want)
	}
}
