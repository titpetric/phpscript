package test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/titpetric/phpscript/tests"
)

func matrixSample() matrixRow {
	return matrixRow{
		DisplayPath: "a.phpt",
		Cells: []matrixCell{
			{Runner: tests.RunnerFlatstack, Status: matrixPass},
			{Runner: tests.RunnerRuntime, Status: matrixFail, Reason: "output mismatch:\n  got:  \"x\"\n  want: \"y\""},
			{Runner: tests.RunnerPHP, Status: matrixSkip, Reason: "opted out by runner.php: false"},
		},
	}
}

func TestTerminalMatrixColumnsAndSpacing(t *testing.T) {
	var buf bytes.Buffer
	table := newTerminalMatrix(&buf, []string{"a-much-longer-name.phpt"}, Options{})
	table.writeHeader()
	table.writeRow(matrixSample())
	table.close()

	output := ansi.Strip(buf.String())
	lines := strings.Split(strings.TrimSpace(output), "\n")
	header := strings.Join(strings.Fields(strings.ReplaceAll(lines[1], "│", "|")), " ")
	if want := "| Fixture | Flat stack | Runtime | PHP |"; header != want {
		t.Errorf("header = %q, want %q", header, want)
	}
	for i, line := range lines {
		if ansi.StringWidth(line) != ansi.StringWidth(lines[0]) {
			t.Errorf("line %d has width %d, want %d:\n%s", i, ansi.StringWidth(line), ansi.StringWidth(lines[0]), output)
		}
	}
	if !strings.Contains(buf.String(), colorGreen+"PASS") ||
		!strings.Contains(buf.String(), colorRed+"FAIL") ||
		!strings.Contains(buf.String(), colorDim+"SKIP") {
		t.Errorf("table is missing expected ANSI colors: %q", buf.String())
	}
	// Without --verbose a failure reason stays out of the table.
	if strings.Contains(output, "output mismatch") {
		t.Errorf("non-verbose table printed a failure reason:\n%s", output)
	}
}

func TestTerminalMatrixVerboseContinuationRows(t *testing.T) {
	var buf bytes.Buffer
	table := newTerminalMatrix(&buf, []string{"a.phpt"}, Options{Verbose: true})
	table.writeHeader()
	table.writeRow(matrixSample())
	table.close()

	output := ansi.Strip(buf.String())
	lines := strings.Split(strings.TrimSpace(output), "\n")
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
	table := newMatrixTable(&buf, []string{"a.phpt"}, Options{Verbose: true})
	table.writeHeader()
	table.writeRow(matrixSample())
	table.close()
	table.writeSummary(0, 1, 1, time.Millisecond)

	got := buf.String()
	if strings.Contains(got, boxVertical) || strings.Contains(got, colorGreen) {
		t.Errorf("piped output is not markdown: %q", got)
	}
	for _, want := range []string{
		"| Fixture | Flat stack | Runtime | PHP  |",
		"| a.phpt  | PASS       | FAIL    | SKIP |",
		"runtime: output mismatch:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown table is missing %q:\n%s", want, got)
		}
	}
	// Every markdown row has one cell per column, continuation rows included.
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if columns := strings.Count(line, "|"); columns != 5 {
			t.Errorf("row %q has %d separators, want 5", line, columns)
		}
	}
}

func TestMatrixTableFitWidensLastColumn(t *testing.T) {
	var buf bytes.Buffer
	table := newTerminalMatrix(&buf, []string{"a.phpt"}, Options{Verbose: true})
	natural := table.detailWidth()
	table.fit(maxTableWidth)
	if table.detailWidth() <= natural {
		t.Errorf("detail width = %d, want more than the natural %d", table.detailWidth(), natural)
	}
	table.writeHeader()
	table.close()
	lines := strings.Split(strings.TrimSpace(ansi.Strip(buf.String())), "\n")
	if width := ansi.StringWidth(lines[0]); width != maxTableWidth {
		t.Errorf("table width = %d, want %d", width, maxTableWidth)
	}
	// A table already wider than the terminal is left alone.
	before := table.detailWidth()
	table.fit(10)
	if table.detailWidth() != before {
		t.Errorf("detail width = %d, want it unchanged at %d", table.detailWidth(), before)
	}
}
