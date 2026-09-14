package test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/titpetric/phpscript/internal/table"
	"github.com/titpetric/phpscript/tests"
)

func matrixSample() matrixRow {
	return matrixRow{
		DisplayPath: "arrays/a.phpt",
		Label:       "a.phpt",
		Cells: []matrixCell{
			{Runner: tests.RunnerFlatstack, Status: matrixPass},
			{Runner: tests.RunnerRuntime, Status: matrixFail, Reason: "output mismatch:\n  got:  \"x\"\n  want: \"y\""},
			{Runner: tests.RunnerPHP, Status: matrixSkip, Reason: "opted out by runner.php: false"},
		},
	}
}

func TestTerminalMatrixColumnsAndSpacing(t *testing.T) {
	var buf bytes.Buffer
	tbl := newTerminalMatrix(&buf, Options{})
	tbl.writeGroup("arrays", []string{"a-much-longer-name.phpt"})
	tbl.writeRow(matrixSample())
	tbl.closeGroup(groupTotals{Dir: "arrays", Failed: 1, Total: 1})

	output := ansi.Strip(buf.String())
	// The trailing lines are the folder subtotal and its blank line.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	lines = lines[:len(lines)-1]
	header := strings.Join(strings.Fields(strings.ReplaceAll(lines[1], "│", "|")), " ")
	if want := "| arrays | Flat stack | Runtime | PHP |"; header != want {
		t.Errorf("header = %q, want %q", header, want)
	}
	for i, line := range lines {
		if ansi.StringWidth(line) != ansi.StringWidth(lines[0]) {
			t.Errorf("line %d has width %d, want %d:\n%s", i, ansi.StringWidth(line), ansi.StringWidth(lines[0]), output)
		}
	}
	if !strings.Contains(buf.String(), table.ColorGreen+"PASS") ||
		!strings.Contains(buf.String(), table.ColorRed+"FAIL") ||
		!strings.Contains(buf.String(), table.ColorDim+"SKIP") {
		t.Errorf("table is missing expected ANSI colors: %q", buf.String())
	}
	// Without --verbose a failure reason stays out of the table.
	if strings.Contains(output, "output mismatch") {
		t.Errorf("non-verbose table printed a failure reason:\n%s", output)
	}
}

func TestSkipPHPDropsTheColumn(t *testing.T) {
	if got, want := len(Options{SkipPHP: true}.runners()), len(tests.Runners)-1; got != want {
		t.Fatalf("runners() length = %d, want %d", got, want)
	}
	for _, r := range (Options{SkipPHP: true}).runners() {
		if r == tests.RunnerPHP {
			t.Fatalf("runners() still lists %s", tests.RunnerPHP)
		}
	}

	var buf bytes.Buffer
	tbl := newTerminalMatrix(&buf, Options{SkipPHP: true})
	tbl.writeGroup("arrays", []string{"a.phpt"})
	row := matrixSample()
	// The cells mirror the run loop, which walks the same filtered list.
	row.Cells = row.Cells[:2]
	tbl.writeRow(row)
	tbl.closeGroup(groupTotals{Dir: "arrays", Failed: 1, Total: 1})

	output := ansi.Strip(buf.String())
	lines := strings.Split(strings.TrimSpace(output), "\n")
	header := strings.Join(strings.Fields(strings.ReplaceAll(lines[1], "│", "|")), " ")
	if want := "| arrays | Flat stack | Runtime |"; header != want {
		t.Errorf("header = %q, want %q", header, want)
	}
	if strings.Contains(output, "PHP") {
		t.Errorf("skip-php table still shows a PHP column:\n%s", output)
	}
}

func TestTerminalMatrixVerboseContinuationRows(t *testing.T) {
	var buf bytes.Buffer
	tbl := newTerminalMatrix(&buf, Options{Verbose: true})
	tbl.writeGroup("arrays", []string{"a.phpt"})
	tbl.writeRow(matrixSample())
	tbl.closeGroup(groupTotals{Dir: "arrays", Failed: 1, Total: 1})

	output := ansi.Strip(buf.String())
	lines := strings.Split(strings.TrimSpace(output), "\n")
	lines = lines[:len(lines)-1]
	for i, line := range lines {
		if ansi.StringWidth(line) != ansi.StringWidth(lines[0]) {
			t.Errorf("line %d has width %d, want %d:\n%s", i, ansi.StringWidth(line), ansi.StringWidth(lines[0]), output)
		}
	}

	var details []string
	for _, line := range lines {
		if strings.HasPrefix(line, "│") && strings.HasPrefix(strings.TrimSpace(strings.Split(line, "│")[1]), "") &&
			strings.Contains(line, "runtime: ") {
			details = append(details, line)
		}
	}
	if len(details) != 1 {
		t.Fatalf("want one continuation row naming the failed runner:\n%s", output)
	}
	// The fixture column of a continuation row is empty, so the row reads as
	// part of the fixture above it.
	if fixture := strings.Split(details[0], "│")[1]; strings.TrimSpace(fixture) != "" {
		t.Errorf("continuation row fixture column = %q, want empty", fixture)
	}
	if !strings.Contains(output, "want: \"y\"") {
		t.Errorf("continuation rows dropped part of the reason:\n%s", output)
	}
	// A skipped runner is not a failure, so it contributes no detail.
	if strings.Contains(output, "opted out") {
		t.Errorf("skip reason printed as a failure:\n%s", output)
	}
}

func TestMatrixTableFallsBackToMarkdown(t *testing.T) {
	var buf bytes.Buffer
	tbl := newMatrixTable(&buf, Options{Verbose: true}, true)
	tbl.writeGroup("arrays", []string{"a.phpt"})
	tbl.writeRow(matrixSample())
	tbl.closeGroup(groupTotals{Dir: "arrays", Failed: 1, Total: 1})
	rows := strings.Split(strings.TrimSpace(buf.String()), "\n")
	tbl.writeSummary(0, 1, 1, time.Millisecond, nil)

	got := buf.String()
	if strings.Contains(got, table.BoxVertical) || strings.Contains(got, table.ColorGreen) {
		t.Errorf("piped output is not markdown: %q", got)
	}
	for _, want := range []string{
		"## arrays",
		"| arrays | Flat stack | Runtime | PHP  |",
		"| a.phpt | PASS       | FAIL    | SKIP |",
		"runtime: output mismatch:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown table is missing %q:\n%s", want, got)
		}
	}
	// Every markdown table row has one cell per column, continuation rows
	// included. The heading and blank lines around them are not table rows.
	for _, line := range rows {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if columns := strings.Count(line, "|"); columns != 5 {
			t.Errorf("row %q has %d separators, want 5", line, columns)
		}
	}
}

func TestMatrixTableFitWidensLastColumn(t *testing.T) {
	var buf bytes.Buffer
	tbl := newTerminalMatrix(&buf, Options{Verbose: true})
	tbl.termWidth = maxTableWidth
	tbl.sizeColumns("arrays", []string{"a.phpt"})
	natural := tbl.detailWidth()
	tbl.fit(maxTableWidth)
	if tbl.detailWidth() <= natural {
		t.Errorf("detail width = %d, want more than the natural %d", tbl.detailWidth(), natural)
	}
	tbl.writeGroup("arrays", []string{"a.phpt"})
	lines := strings.Split(strings.TrimSpace(ansi.Strip(buf.String())), "\n")
	if width := ansi.StringWidth(lines[0]); width != maxTableWidth {
		t.Errorf("table width = %d, want %d", width, maxTableWidth)
	}
	// A table already wider than the terminal is left alone.
	before := tbl.detailWidth()
	tbl.fit(10)
	if tbl.detailWidth() != before {
		t.Errorf("detail width = %d, want it unchanged at %d", tbl.detailWidth(), before)
	}
	// A second folder re-fits from the header widths rather than compounding:
	// fit adds to the last column, so a writeGroup that forgot to reset the
	// widths would grow the table past the terminal on every folder.
	buf.Reset()
	tbl.writeGroup("strings", []string{"b.phpt"})
	lines = strings.Split(strings.TrimSpace(ansi.Strip(buf.String())), "\n")
	if width := ansi.StringWidth(lines[0]); width != maxTableWidth {
		t.Errorf("second folder width = %d, want %d", width, maxTableWidth)
	}
}

// TestMatrixDurationsSplitPerEngine covers the per-runner share the matrix
// subtotal and summary lines carry after the wall-clock figure.
func TestMatrixDurationsSplitPerEngine(t *testing.T) {
	engines := []engineDuration{
		{Runner: tests.RunnerFlatstack, Duration: time.Millisecond},
		{Runner: tests.RunnerRuntime, Duration: 2 * time.Millisecond},
		{Runner: tests.RunnerPHP, Duration: 3 * time.Millisecond},
	}

	var buf bytes.Buffer
	tbl := newTerminalMatrix(&buf, Options{})
	tbl.sizeColumns("arrays", []string{"a.phpt"})
	tbl.closeGroup(groupTotals{Dir: "arrays", Passed: 1, Total: 1, Duration: 6 * time.Millisecond, Engines: engines})
	tbl.writeSummary(1, 0, 1, 6*time.Millisecond, engines)

	output := ansi.Strip(buf.String())
	for _, want := range []string{
		"arrays: 1 passed, 0 failed out of 1 fixtures (6ms: flatstack 1ms, runtime 2ms, php 3ms)",
		"Matrix summary: 1 passed, 0 failed out of 1 fixtures (6ms: flatstack 1ms, runtime 2ms, php 3ms)",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output is missing %q:\n%s", want, output)
		}
	}
}

// TestFolderTableSplitsEngineDurations covers the folder summary of a matrix
// run with cost columns: one duration column per runner after the wall-clock
// one. Without engines the single column stays, which is what a plain run and
// the checked-in report print.
func TestFolderTableSplitsEngineDurations(t *testing.T) {
	rows := []folderSummary{{groupTotals: groupTotals{
		Dir: "arrays", Passed: 1, Total: 1, Duration: 6 * time.Millisecond,
		Engines: []engineDuration{
			{Runner: tests.RunnerFlatstack, Duration: time.Millisecond},
			{Runner: tests.RunnerRuntime, Duration: 2 * time.Millisecond},
			{Runner: tests.RunnerPHP, Duration: 3 * time.Millisecond},
		},
	}}}

	var buf bytes.Buffer
	writeFolderTable(&buf, rows, true, true)
	got := buf.String()
	for _, want := range []string{
		"| Path   | Fixtures | Passed | Failed | Duration (ms) | Flat stack (ms) | Runtime (ms) | PHP (ms) |",
		"| arrays | 1        | 1      | 0      | 6             | 1               | 2            | 3        |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("folder table is missing %q:\n%s", want, got)
		}
	}

	buf.Reset()
	rows[0].Engines = nil
	writeFolderTable(&buf, rows, true, true)
	if got := buf.String(); !strings.Contains(got, "| Path   | Fixtures | Passed | Failed | Duration (ms) |") {
		t.Errorf("plain folder table grew engine columns:\n%s", got)
	}
}

// TestMarkdownSummarySplitsEngineDurations covers the closing summary table of
// a -o matrix report with cost columns.
func TestMarkdownSummarySplitsEngineDurations(t *testing.T) {
	engines := []engineDuration{
		{Runner: tests.RunnerFlatstack, Duration: time.Millisecond},
		{Runner: tests.RunnerPHP, Duration: 3 * time.Millisecond},
	}

	var buf bytes.Buffer
	tbl := newMarkdownMatrix(&buf, Options{Profile: true})
	tbl.closeGroup(groupTotals{Dir: "arrays", Passed: 1, Total: 1, Duration: 4 * time.Millisecond, Engines: engines})
	tbl.writeSummary(1, 0, 1, 4*time.Millisecond, engines)

	got := buf.String()
	for _, want := range []string{
		"| Area      | Fixtures | Passed | Failed | Duration (ms) | Flat stack (ms) | PHP (ms) |",
		"| arrays    | 1        | 1      | 0      | 4             | 1               | 3        |",
		"| **Total** | 1        | 1      | 0      | 4             | 1               | 3        |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary table is missing %q:\n%s", want, got)
		}
	}
}

// TestMatrixMetricColumnsFollowProfileFlags pins the rule that keeps the
// generated report stable: a plain --matrix run has no cost columns.
func TestMatrixMetricColumnsFollowProfileFlags(t *testing.T) {
	for _, c := range []struct {
		opts Options
		want bool
	}{
		{Options{}, false},
		{Options{Profile: true}, true},
		{Options{Count: 2}, true},
	} {
		var buf bytes.Buffer
		tbl := newMatrixTable(&buf, c.opts, true)
		tbl.writeGroup("arrays", []string{"a.phpt"})
		got := strings.Contains(buf.String(), "GC Runs")
		if got != c.want {
			t.Errorf("opts %+v: metric columns = %v, want %v:\n%s", c.opts, got, c.want, buf.String())
		}
	}
}
