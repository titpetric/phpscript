package test

import (
	"fmt"
	"io"
	"os"
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
	writeHeader()
	writeRow(matrixRow)
	close()
	writeSummary(passed, failed, total int, duration time.Duration)
}

// terminalMatrix writes the ansi table. Column widths are established before
// the run so a fixture can be printed as soon as it completes.
type terminalMatrix struct {
	w       io.Writer
	headers []string
	widths  []int
	verbose bool
}

func newMatrixTable(w io.Writer, filenames []string, opts Options) matrixTable {
	table := newTerminalMatrix(w, filenames, opts)
	file, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return &markdownMatrix{terminalMatrix: table}
	}
	if width, _, err := term.GetSize(int(file.Fd())); err == nil {
		table.fit(width)
	}
	return table
}

// fit widens the last column so a verbose table fills the terminal. The failure
// detail below a row is wrapped into the runner columns, and the natural width
// of three status columns leaves too little of a reason to read.
func (t *terminalMatrix) fit(width int) {
	if !t.verbose {
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

func newTerminalMatrix(w io.Writer, filenames []string, opts Options) *terminalMatrix {
	t := &terminalMatrix{w: w, verbose: opts.Verbose}
	t.headers = []string{"Fixture"}
	for _, runner := range tests.Runners {
		t.headers = append(t.headers, matrixHeaders[runner])
	}

	t.widths = make([]int, len(t.headers))
	for i, header := range t.headers {
		t.widths[i] = ansi.StringWidth(header)
	}
	for _, filename := range filenames {
		if width := ansi.StringWidth(filename); width > t.widths[0] {
			t.widths[0] = width
		}
	}
	for i := 1; i < len(t.widths); i++ {
		t.widths[i] = max(t.widths[i], len("PASS"))
	}
	return t
}

func (t *terminalMatrix) writeHeader() {
	t.writeBorder(boxTopLeft, boxTeeDown, boxTopRight)
	t.writeRowValues(t.headers, colorHeader)
	t.writeBorder(boxTeeRight, boxCross, boxTeeLeft)
}

func (t *terminalMatrix) writeRow(row matrixRow) {
	values := []string{colorAmber + row.DisplayPath + colorReset}
	for _, cell := range row.Cells {
		values = append(values, statusColor(cell.Status)+statusLabel(cell.Status)+colorReset)
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

func (t *terminalMatrix) close() {
	t.writeBorder(boxBottomLeft, boxTeeUp, boxBottomRight)
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

// markdownMatrix renders the same matrix for a piped stdout.
type markdownMatrix struct {
	*terminalMatrix
}

func (t *markdownMatrix) writeHeader() {
	t.writeMarkdownRow(t.headers)
	separators := make([]string, len(t.widths))
	for i, width := range t.widths {
		separators[i] = strings.Repeat("-", width)
	}
	t.writeMarkdownRow(separators)
}

func (t *markdownMatrix) writeRow(row matrixRow) {
	values := []string{row.DisplayPath}
	for _, cell := range row.Cells {
		values = append(values, statusLabel(cell.Status))
	}
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

func (t *markdownMatrix) close() {}

func (t *markdownMatrix) writeSummary(int, int, int, time.Duration) {}

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
