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
	count   int
	loop    bool
	profile bool
	lat     bool
}

type resultTable interface {
	writeGroup(dir string, labels []string)
	writeResult(*fixtureRun)
	closeGroup(groupTotals)
	writeSummary(passed, failed, total int, duration time.Duration)
}

// isTerminal reports whether w is a tty, which is what decides between the
// box-drawing table and markdown when the caller has not said which it wants.
func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func newResultTable(w io.Writer, opts Options, markdown bool) resultTable {
	if markdown {
		return newMarkdownTable(w, opts)
	}
	return newTerminalTable(w, opts)
}

func newTerminalTable(w io.Writer, opts Options) *terminalTable {
	return &terminalTable{
		w:       w,
		count:   opts.Count,
		loop:    opts.Count > 0 || opts.Time > 0,
		profile: opts.Profile,
		lat:     opts.Time > 0,
	}
}

// sizeColumns rebuilds the header row and the column widths for one folder.
// The folder name is the header of the filename column, so a table says which
// area it covers without spending a line on a title.
func (t *terminalTable) sizeColumns(dir string, labels []string) {
	if dir == "" {
		dir = "Filename"
	}
	t.headers = []string{"Test", dir, "Duration (ms)"}
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

	t.widths = make([]int, len(t.headers))
	for i, header := range t.headers {
		t.widths[i] = ansi.StringWidth(header)
	}
	for _, label := range labels {
		if width := ansi.StringWidth(label); width > t.widths[1] {
			t.widths[1] = width
		}
	}
	if t.loop {
		t.widths[3] = max(t.widths[3], 7, len(strconv.Itoa(t.count)))
	}
	t.widths[len(t.widths)-1] = max(t.widths[len(t.widths)-1], 15)
}

func (t *terminalTable) writeGroup(dir string, labels []string) {
	t.sizeColumns(dir, labels)
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
		colorAmber + r.label() + colorReset,
		colorWhite + formatDuration(r.Total) + colorReset,
	}
	if t.loop {
		row = append(row, colorWhite+strconv.Itoa(r.Runs)+colorReset)
	}
	if t.lat {
		row = append(row,
			colorWhite+formatMicros(r.P50)+colorReset,
			colorWhite+formatMicros(r.P95)+colorReset,
			colorWhite+formatMicros(r.P99)+colorReset,
		)
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

func (t *terminalTable) closeGroup(totals groupTotals) {
	t.writeBorder(boxBottomLeft, boxTeeUp, boxBottomRight)
	fmt.Fprintf(t.w, "%s%s: %d passed, %d failed out of %d fixtures (%dms)%s\n\n",
		colorHeader, totals.Dir, totals.Passed, totals.Failed, totals.Total,
		totals.Duration.Milliseconds(), colorReset)
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
	totals []groupTotals
}

func newMarkdownTable(w io.Writer, opts Options) *markdownTable {
	return &markdownTable{terminalTable: newTerminalTable(w, opts)}
}

func (t *markdownTable) writeGroup(dir string, labels []string) {
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

func (t *markdownTable) closeGroup(totals groupTotals) {
	t.totals = append(t.totals, totals)
	fmt.Fprintln(t.w)
}

func (t *markdownTable) writeResult(r *fixtureRun) {
	status := "PASS"
	if !r.Result.Passed {
		status = "FAIL"
	}
	row := []string{status, markdownCell(r.label()), formatDuration(r.Total)}
	if t.loop {
		row = append(row, strconv.Itoa(r.Runs))
	}
	if t.lat {
		row = append(row, formatMicros(r.P50), formatMicros(r.P95), formatMicros(r.P99))
	}
	if t.profile {
		row = append(row, strconv.FormatUint(r.AllocsPerOp, 10), strconv.FormatUint(r.BytesPerOp, 10))
	}
	row = append(row, formatGCRuns(r.GCRuns, r.Runs))
	t.writeMarkdownRow(row)
}

// writeSummary closes the report with the per-folder totals, so the reader
// sees the aggregate without adding up the tables above it.
func (t *markdownTable) writeSummary(passed, failed, total int, duration time.Duration) {
	writeMarkdownSummary(t.w, t.totals, passed, failed, total, duration, t.lat || t.loop || t.profile)
}

// writeMarkdownSummary renders the per-folder totals table that closes a
// report. The Total row is the sum of the folder rows and equals the counts
// the caller passed, which is what makes the report self-checking.
//
// The duration column appears only when the run asked for cost columns. A
// checked-in report is regenerated by every pipeline run, and a timing that
// differs by a millisecond each time would show up as a diff in every commit.
func writeMarkdownSummary(w io.Writer, totals []groupTotals, passed, failed, total int, duration time.Duration, timings bool) {
	headers := []string{"Area", "Fixtures", "Passed", "Failed"}
	widths := []int{len("**Total**"), len("Fixtures"), len("Passed"), len("Failed")}
	if timings {
		headers = append(headers, "Duration (ms)")
		widths = append(widths, len("Duration (ms)"))
	}
	for _, group := range totals {
		if width := ansi.StringWidth(group.Dir); width > widths[0] {
			widths[0] = width
		}
	}

	row := func(values []string) {
		cells := make([]string, len(values))
		for i, value := range values {
			cells[i] = value + strings.Repeat(" ", max(0, widths[i]-ansi.StringWidth(value)))
		}
		fmt.Fprintln(w, "| "+strings.Join(cells, " | ")+" |")
	}

	fmt.Fprint(w, "## Summary\n\n")
	row(headers)
	separators := make([]string, len(widths))
	for i, width := range widths {
		separators[i] = strings.Repeat("-", width)
	}
	row(separators)
	for _, group := range totals {
		values := []string{
			group.Dir,
			strconv.Itoa(group.Total),
			strconv.Itoa(group.Passed),
			strconv.Itoa(group.Failed),
		}
		if timings {
			values = append(values, formatDuration(group.Duration))
		}
		row(values)
	}
	values := []string{
		"**Total**",
		strconv.Itoa(total),
		strconv.Itoa(passed),
		strconv.Itoa(failed),
	}
	if timings {
		values = append(values, formatDuration(duration))
	}
	row(values)
	fmt.Fprintln(w)
}

// markdownCell keeps a value inside its table cell: a pipe would end the cell
// and a newline would end the row.
func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.Join(strings.Fields(value), " ")
}

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

func formatMicros(d time.Duration) string {
	return strconv.FormatInt(d.Microseconds(), 10)
}

func formatGCRuns(gcRuns uint32, runs int) string {
	percentage := 0.0
	if runs > 0 {
		percentage = float64(gcRuns) * 100 / float64(runs)
	}
	return fmt.Sprintf("%d (%.2f%%)", gcRuns, percentage)
}
