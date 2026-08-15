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
)

const (
	boxTopLeft     = "╭"
	boxTopRight    = "╮"
	boxBottomLeft  = "╰"
	boxBottomRight = "╯"
	boxHorizontal  = "─"
	boxVertical    = "│"
	boxTeeDown     = "┬"
	boxTeeUp       = "┴"
	boxTeeRight    = "├"
	boxTeeLeft     = "┤"
	boxCross       = "┼"

	colorReset     = "\033[0m"
	colorSeparator = "\033[38;5;238m"
	colorHeader    = "\033[38;5;146m"
	colorAmber     = "\033[38;5;214m"
	colorGreen     = "\033[38;5;114m"
	colorWhite     = "\033[38;5;255m"
	colorRed       = "\033[38;5;167m"
)

// terminalTable writes one padded row at a time. Column widths are established
// before tests start so completed fixtures can be printed immediately.
type terminalTable struct {
	w       io.Writer
	headers []string
	widths  []int
	loop    bool
	profile bool
}

type resultTable interface {
	writeHeader()
	writeResult(*fixtureRun)
	close()
	writeSummary(passed, failed, total int, duration time.Duration)
}

func newResultTable(w io.Writer, filenames []string, opts Options) resultTable {
	if file, ok := w.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		return newTerminalTable(w, filenames, opts)
	}
	return newMarkdownTable(w, filenames, opts)
}

func newTerminalTable(w io.Writer, filenames []string, opts Options) *terminalTable {
	t := &terminalTable{
		w:       w,
		headers: []string{"Test", "Filename", "Duration (ms)"},
		loop:    opts.Count > 0 || opts.Time > 0,
		profile: opts.Profile,
	}
	if t.loop {
		t.headers = append(t.headers, "Count")
	}
	if t.profile {
		t.headers = append(t.headers, "Allocs/op", "Bytes/op")
	}
	t.headers = append(t.headers, "GC Runs")

	t.widths = make([]int, len(t.headers))
	for i, header := range t.headers {
		t.widths[i] = ansi.StringWidth(header)
	}
	for _, filename := range filenames {
		if width := ansi.StringWidth(filename); width > t.widths[1] {
			t.widths[1] = width
		}
	}
	if t.loop {
		t.widths[3] = max(t.widths[3], 7, len(strconv.Itoa(opts.Count)))
	}
	t.widths[len(t.widths)-1] = max(t.widths[len(t.widths)-1], 15)
	return t
}

func (t *terminalTable) writeHeader() {
	t.writeBorder(boxTopLeft, boxTeeDown, boxTopRight)
	t.writeRow(t.headers, colorHeader)
	t.writeBorder(boxTeeRight, boxCross, boxTeeLeft)
}

func (t *terminalTable) writeResult(r *fixtureRun) {
	status := colorGreen + "PASS" + colorReset
	if !r.Result.Passed {
		status = colorRed + "FAIL" + colorReset
	}
	row := []string{
		status,
		colorAmber + r.DisplayPath + colorReset,
		colorWhite + formatDuration(r.Total) + colorReset,
	}
	if t.loop {
		row = append(row, colorWhite+strconv.Itoa(r.Runs)+colorReset)
	}
	if t.profile {
		row = append(row,
			colorWhite+strconv.FormatUint(r.AllocsPerOp, 10)+colorReset,
			colorWhite+strconv.FormatUint(r.BytesPerOp, 10)+colorReset,
		)
	}
	row = append(row, colorWhite+formatGCRuns(r.GCRuns, r.Runs)+colorReset)
	t.writeRow(row, "")
}

func (t *terminalTable) close() {
	t.writeBorder(boxBottomLeft, boxTeeUp, boxBottomRight)
}

func (t *terminalTable) writeSummary(passed, failed, total int, duration time.Duration) {
	fmt.Fprintf(t.w, "%sTest summary: %d passed, %d failed out of %d fixtures (%dms)%s\n",
		colorHeader, passed, failed, total, duration.Milliseconds(), colorReset)
}

func (t *terminalTable) writeBorder(left, middle, right string) {
	segments := make([]string, len(t.widths))
	for i, width := range t.widths {
		segments[i] = strings.Repeat(boxHorizontal, width+2)
	}
	fmt.Fprintln(t.w, colorSeparator+left+strings.Join(segments, middle)+right+colorReset)
}

func (t *terminalTable) writeRow(values []string, color string) {
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

type markdownTable struct {
	*terminalTable
}

func newMarkdownTable(w io.Writer, filenames []string, opts Options) *markdownTable {
	return &markdownTable{terminalTable: newTerminalTable(w, filenames, opts)}
}

func (t *markdownTable) writeHeader() {
	t.writeMarkdownRow(t.headers)
	separators := make([]string, len(t.widths))
	for i, width := range t.widths {
		separators[i] = strings.Repeat("-", width)
	}
	t.writeMarkdownRow(separators)
}

func (t *markdownTable) writeResult(r *fixtureRun) {
	status := "PASS"
	if !r.Result.Passed {
		status = "FAIL"
	}
	row := []string{status, r.DisplayPath, formatDuration(r.Total)}
	if t.loop {
		row = append(row, strconv.Itoa(r.Runs))
	}
	if t.profile {
		row = append(row, strconv.FormatUint(r.AllocsPerOp, 10), strconv.FormatUint(r.BytesPerOp, 10))
	}
	row = append(row, formatGCRuns(r.GCRuns, r.Runs))
	t.writeMarkdownRow(row)
}

func (t *markdownTable) close() {}

func (t *markdownTable) writeSummary(int, int, int, time.Duration) {}

func (t *markdownTable) writeMarkdownRow(values []string) {
	cells := make([]string, len(values))
	for i, value := range values {
		cells[i] = value + strings.Repeat(" ", max(0, t.widths[i]-ansi.StringWidth(value)))
	}
	fmt.Fprintln(t.w, "| "+strings.Join(cells, " | ")+" |")
}

func formatDuration(duration time.Duration) string {
	return strconv.FormatInt(duration.Milliseconds(), 10)
}

func formatGCRuns(gcRuns uint32, runs int) string {
	percentage := 0.0
	if runs > 0 {
		percentage = float64(gcRuns) * 100 / float64(runs)
	}
	return fmt.Sprintf("%d (%.2f%%)", gcRuns, percentage)
}
