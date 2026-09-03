package test

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/titpetric/phpscript/internal/table"
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
	cover   bool
}

type resultTable interface {
	writeGroup(dir string, labels []string)
	writeResult(*fixtureRun)
	closeGroup(groupTotals)
	writeSummary(passed, failed, total int, duration time.Duration)
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
		cover:   opts.Cover != "",
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
	if t.cover {
		t.headers = append(t.headers, "Coverage")
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
	t.writeBorder(table.BoxTopLeft, table.BoxTeeDown, table.BoxTopRight)
	t.writeRow(t.headers, table.ColorHeader)
	t.writeBorder(table.BoxTeeRight, table.BoxCross, table.BoxTeeLeft)
}

func (t *terminalTable) writeResult(r *fixtureRun) {
	status := table.ColorGreen + "PASS" + table.ColorReset
	switch {
	case r.Result.Skipped:
		status = table.ColorDim + "SKIP" + table.ColorReset
	case !r.Result.Passed:
		status = table.ColorRed + "FAIL" + table.ColorReset
	}
	row := []string{
		status,
		table.ColorAmber + r.label() + table.ColorReset,
		table.ColorWhite + formatDuration(r.Total) + table.ColorReset,
	}
	if t.loop {
		row = append(row, table.ColorWhite+strconv.Itoa(r.Runs)+table.ColorReset)
	}
	if t.lat {
		row = append(row,
			table.ColorWhite+formatMicros(r.P50)+table.ColorReset,
			table.ColorWhite+formatMicros(r.P95)+table.ColorReset,
			table.ColorWhite+formatMicros(r.P99)+table.ColorReset,
		)
	}
	if t.profile {
		row = append(row,
			table.ColorWhite+strconv.FormatUint(r.AllocsPerOp, 10)+table.ColorReset,
			table.ColorWhite+strconv.FormatUint(r.BytesPerOp, 10)+table.ColorReset,
		)
	}
	if t.cover {
		row = append(row, table.ColorWhite+fixtureCoverage(r.Fixture)+table.ColorReset)
	}
	row = append(row, table.ColorWhite+formatGCRuns(r.GCRuns, r.Runs)+table.ColorReset)
	t.writeRow(row, "")
}

func (t *terminalTable) closeGroup(totals groupTotals) {
	t.writeBorder(table.BoxBottomLeft, table.BoxTeeUp, table.BoxBottomRight)
	fmt.Fprintf(t.w, "%s%s: %d passed, %d failed out of %d fixtures (%dms)%s\n\n",
		table.ColorHeader, totals.Dir, totals.Passed, totals.Failed, totals.Total,
		totals.Duration.Milliseconds(), table.ColorReset)
}

func (t *terminalTable) writeSummary(passed, failed, total int, duration time.Duration) {
	fmt.Fprintf(t.w, "%sTest summary: %d passed, %d failed out of %d fixtures (%dms)%s\n",
		table.ColorHeader, passed, failed, total, duration.Milliseconds(), table.ColorReset)
}

func (t *terminalTable) writeBorder(left, middle, right string) {
	segments := make([]string, len(t.widths))
	for i, width := range t.widths {
		segments[i] = strings.Repeat(table.BoxHorizontal, width+2)
	}
	fmt.Fprintln(t.w, table.ColorSeparator+left+strings.Join(segments, middle)+right+table.ColorReset)
}

func (t *terminalTable) writeRow(values []string, color string) {
	cells := make([]string, len(values))
	reset := ""
	if color != "" {
		reset = table.ColorReset
	}
	for i, value := range values {
		padding := strings.Repeat(" ", max(0, t.widths[i]-ansi.StringWidth(value)))
		cells[i] = " " + color + value + padding + reset + " "
	}
	separator := table.ColorSeparator + table.BoxVertical + table.ColorReset
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
	switch {
	case r.Result.Skipped:
		status = "SKIP"
	case !r.Result.Passed:
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
	if t.cover {
		row = append(row, fixtureCoverage(r.Fixture))
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
