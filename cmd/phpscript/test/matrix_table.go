package test

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	"github.com/titpetric/phpscript/tests"
)

const colorDim = "\033[38;5;244m"

// maxTableWidth bounds how far a verbose table is widened to fill a terminal.
const maxTableWidth = 120

// matrixHeaders labels each runner column of the matrix report.
var matrixHeaders = map[tests.Runner]string{
	tests.RunnerFlatstack: "Flat stack",
	tests.RunnerRuntime:   "Runtime",
	tests.RunnerPHP:       "PHP",
}

// matrixTable renders one fixture per row with a cell per runner.
type matrixTable interface {
	writeGroup(dir string, labels []string)
	writeRow(matrixRow)
	closeGroup(groupTotals)
	writeSummary(passed, failed, total int, duration time.Duration)
}

// terminalMatrix writes the ansi table. Column widths are established per
// folder so a fixture can be printed as soon as it completes.
type terminalMatrix struct {
	w         io.Writer
	headers   []string
	widths    []int
	termWidth int
	verbose   bool
	count     int
	loop      bool
	profile   bool
	lat       bool
}

func newMatrixTable(w io.Writer, opts Options, markdown bool) matrixTable {
	table := newTerminalMatrix(w, opts)
	if markdown {
		return &markdownMatrix{terminalMatrix: table}
	}
	if file, ok := w.(*os.File); ok {
		if width, _, err := term.GetSize(int(file.Fd())); err == nil {
			table.termWidth = width
		}
	}
	return table
}

// fit widens the last column so a verbose table fills the terminal. The failure
// detail below a row is wrapped into the runner columns, and the natural width
// of three status columns leaves too little of a reason to read.
//
// It is applied after the widths have been rebuilt for a folder, never before:
// fit adds to the last column, so applying it to already-fitted widths would
// compound across folders.
func (t *terminalMatrix) fit(width int) {
	if !t.verbose || width == 0 {
		return
	}
	natural := 1
	for _, w := range t.widths {
		natural += w + 3
	}
	if grow := min(width, maxTableWidth) - natural; grow > 0 {
		t.widths[len(t.widths)-1] += grow
	}
}

func newTerminalMatrix(w io.Writer, opts Options) *terminalMatrix {
	return &terminalMatrix{
		w:       w,
		verbose: opts.Verbose,
		count:   opts.Count,
		loop:    opts.Count > 0 || opts.Time > 0,
		profile: opts.Profile,
		lat:     opts.Time > 0,
	}
}

// metrics reports whether the run asked for cost columns. Without a
// benchmarking flag the matrix stays exactly as wide as it is without one,
// which is what keeps the generated report stable.
func (t *terminalMatrix) metrics() bool {
	return t.loop || t.profile || t.lat
}

// sizeColumns rebuilds the header row and column widths for one folder. The
// folder name is the header of the fixture column, so a table says which area
// it covers without spending a line on a title.
func (t *terminalMatrix) sizeColumns(dir string, labels []string) {
	if dir == "" {
		dir = "Fixture"
	}
	t.headers = []string{dir}
	for _, runner := range tests.Runners {
		t.headers = append(t.headers, matrixHeaders[runner])
	}
	if t.metrics() {
		t.headers = append(t.headers, "Duration (ms)")
		if t.loop {
			t.headers = append(t.headers, "Count")
		}
		if t.lat {
			t.headers = append(t.headers, "P50 (µs)", "P95 (µs)", "P99 (µs)")
		}
		if t.profile {
			t.headers = append(t.headers, "Allocs/op", "Bytes/op")
		}
		t.headers = append(t.headers, "GC Runs")
	}

	t.widths = make([]int, len(t.headers))
	for i, header := range t.headers {
		t.widths[i] = ansi.StringWidth(header)
	}
	for _, label := range labels {
		if width := ansi.StringWidth(label); width > t.widths[0] {
			t.widths[0] = width
		}
	}
	for i := 1; i <= len(tests.Runners); i++ {
		t.widths[i] = max(t.widths[i], len("PASS"))
	}
}

// metricValues renders the cost cells appended after the runner columns.
func (t *terminalMatrix) metricValues(m fixtureMetrics) []string {
	if !t.metrics() {
		return nil
	}
	values := []string{formatDuration(m.Total)}
	if t.loop {
		values = append(values, strconv.Itoa(m.Runs))
	}
	if t.lat {
		values = append(values, formatMicros(m.P50), formatMicros(m.P95), formatMicros(m.P99))
	}
	if t.profile {
		values = append(values,
			strconv.FormatUint(m.AllocsPerOp, 10),
			strconv.FormatUint(m.BytesPerOp, 10))
	}
	return append(values, formatGCRuns(m.GCRuns, m.Runs))
}

func (t *terminalMatrix) writeGroup(dir string, labels []string) {
	t.sizeColumns(dir, labels)
	t.fit(t.termWidth)
	t.writeBorder(boxTopLeft, boxTeeDown, boxTopRight)
	t.writeRowValues(t.headers, colorHeader)
	t.writeBorder(boxTeeRight, boxCross, boxTeeLeft)
}

func (t *terminalMatrix) writeRow(row matrixRow) {
	values := []string{colorAmber + row.label() + colorReset}
	for _, cell := range row.Cells {
		values = append(values, statusColor(cell.Status)+statusLabel(cell.Status)+colorReset)
	}
	for _, value := range t.metricValues(row.Metrics) {
		values = append(values, colorWhite+value+colorReset)
	}
	t.writeRowValues(values, "")

	if !t.verbose {
		return
	}
	for _, cell := range row.Cells {
		if cell.Status != matrixFail || cell.Reason == "" {
			continue
		}
		for _, line := range wrapDetail(string(cell.Runner)+": "+cell.Reason, t.detailWidth()) {
			t.writeDetail(line)
		}
	}
}

func (t *terminalMatrix) closeGroup(totals groupTotals) {
	t.writeBorder(boxBottomLeft, boxTeeUp, boxBottomRight)
	fmt.Fprintf(t.w, "%s%s: %d passed, %d failed out of %d fixtures (%dms)%s\n\n",
		colorHeader, totals.Dir, totals.Passed, totals.Failed, totals.Total,
		totals.Duration.Milliseconds(), colorReset)
}

func (t *terminalMatrix) writeSummary(passed, failed, total int, duration time.Duration) {
	fmt.Fprintf(t.w, "%sMatrix summary: %d passed, %d failed out of %d fixtures (%dms)%s\n",
		colorHeader, passed, failed, total, duration.Milliseconds(), colorReset)
}

// detailWidth is the width of the runner columns merged into one cell, which
// is what a continuation row prints its failure detail in.
func (t *terminalMatrix) detailWidth() int {
	width := 0
	for _, w := range t.widths[1:] {
		width += w
	}
	return width + 3*(len(t.widths)-2)
}

func (t *terminalMatrix) writeDetail(line string) {
	separator := colorSeparator + boxVertical + colorReset
	fixture := strings.Repeat(" ", t.widths[0])
	padding := strings.Repeat(" ", max(0, t.detailWidth()-ansi.StringWidth(line)))
	fmt.Fprintln(t.w, separator+" "+fixture+" "+separator+" "+colorWhite+line+colorReset+padding+" "+separator)
}

func (t *terminalMatrix) writeBorder(left, middle, right string) {
	segments := make([]string, len(t.widths))
	for i, width := range t.widths {
		segments[i] = strings.Repeat(boxHorizontal, width+2)
	}
	fmt.Fprintln(t.w, colorSeparator+left+strings.Join(segments, middle)+right+colorReset)
}

func (t *terminalMatrix) writeRowValues(values []string, color string) {
	cells := make([]string, len(values))
	reset := ""
	if color != "" {
		reset = colorReset
	}
	for i, value := range values {
		padding := strings.Repeat(" ", max(0, t.widths[i]-ansi.StringWidth(value)))
		cells[i] = " " + color + value + padding + reset + " "
	}
	separator := colorSeparator + boxVertical + colorReset
	fmt.Fprintln(t.w, separator+strings.Join(cells, separator)+separator)
}

// markdownMatrix renders the same matrix for a piped stdout or a -o report.
type markdownMatrix struct {
	*terminalMatrix
	totals []groupTotals
}

func newMarkdownMatrix(w io.Writer, opts Options) *markdownMatrix {
	return &markdownMatrix{terminalMatrix: newTerminalMatrix(w, opts)}
}

func (t *markdownMatrix) writeGroup(dir string, labels []string) {
	t.sizeColumns(dir, labels)
	if dir == "" {
		dir = "fixtures"
	}
	fmt.Fprintf(t.w, "## %s\n\n", dir)
	t.writeMarkdownRow(t.headers)
	separators := make([]string, len(t.widths))
	for i, width := range t.widths {
		separators[i] = strings.Repeat("-", width)
	}
	t.writeMarkdownRow(separators)
}

func (t *markdownMatrix) writeRow(row matrixRow) {
	values := []string{markdownCell(row.label())}
	for _, cell := range row.Cells {
		values = append(values, statusLabel(cell.Status))
	}
	values = append(values, t.metricValues(row.Metrics)...)
	t.writeMarkdownRow(values)

	if !t.verbose {
		return
	}
	// Markdown has no fixed width to wrap into, so a reason only breaks where
	// it already carries a newline.
	for _, cell := range row.Cells {
		if cell.Status != matrixFail || cell.Reason == "" {
			continue
		}
		for _, line := range strings.Split(string(cell.Runner)+": "+cell.Reason, "\n") {
			detail := make([]string, len(t.widths))
			detail[1] = strings.ReplaceAll(strings.TrimRight(line, " "), "|", "\\|")
			t.writeMarkdownRow(detail)
		}
	}
}

func (t *markdownMatrix) closeGroup(totals groupTotals) {
	t.totals = append(t.totals, totals)
	fmt.Fprintln(t.w)
}

// writeSummary closes the report with the per-folder totals, so the reader
// sees the aggregate without adding up the tables above it.
func (t *markdownMatrix) writeSummary(passed, failed, total int, duration time.Duration) {
	writeMarkdownSummary(t.w, t.totals, passed, failed, total, duration, t.metrics())
}

func (t *markdownMatrix) writeMarkdownRow(values []string) {
	cells := make([]string, len(values))
	for i, value := range values {
		cells[i] = value + strings.Repeat(" ", max(0, t.widths[i]-ansi.StringWidth(value)))
	}
	fmt.Fprintln(t.w, "| "+strings.Join(cells, " | ")+" |")
}

func statusLabel(status matrixStatus) string {
	switch status {
	case matrixFail:
		return "FAIL"
	case matrixSkip:
		return "SKIP"
	default:
		return "PASS"
	}
}

func statusColor(status matrixStatus) string {
	switch status {
	case matrixFail:
		return colorRed
	case matrixSkip:
		return colorDim
	default:
		return colorGreen
	}
}

// wrapDetail splits a failure reason into lines that fit the detail column.
// Reasons embed newlines to separate the got and want output.
func wrapDetail(reason string, width int) []string {
	var lines []string
	for _, line := range strings.Split(reason, "\n") {
		line = strings.TrimRight(line, " ")
		for ansi.StringWidth(line) > width && width > 0 {
			lines = append(lines, ansi.Truncate(line, width, ""))
			line = strings.TrimPrefix(line, ansi.Truncate(line, width, ""))
		}
		lines = append(lines, line)
	}
	return lines
}
