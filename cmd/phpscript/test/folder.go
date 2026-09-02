package test

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/x/ansi"

	"github.com/titpetric/phpscript/internal/table"
	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/tests"
)

// folderCover is one folder's coverage, in the two counts the summary reports:
// how many of the PHP files its fixtures loaded were reached at all, and how
// many of the statements in them ran.
//
// The two answer different questions. A file count says what the suite has not
// looked at; a statement count says how thoroughly it looked at the rest. A
// folder scoring 8% of files and 90% of lines is tested in one corner, and
// neither number alone would say so.
type folderCover struct {
	Dir          string
	FilesCovered int
	FilesTotal   int
	LinesCovered int
	LinesTotal   int
	// Files is the per-file breakdown, which -v prints below the folder row.
	Files []coverage.Row
}

// files renders the file column: "4/50 (8%)". The header says what is counted,
// so the cell carries the counts and nothing else.
func (f folderCover) files() string {
	if f.FilesTotal == 0 {
		return noCoverage
	}
	return ratio(f.FilesCovered, f.FilesTotal)
}

// lines renders the line column: "1532/8000 (19%)".
func (f folderCover) lines() string {
	if f.LinesTotal == 0 {
		return noCoverage
	}
	return ratio(f.LinesCovered, f.LinesTotal)
}

// ratio is the form both coverage columns take, "N/M (J%)".
func ratio(covered, total int) string {
	return fmt.Sprintf("%d/%d (%d%%)", covered, total, percentOf(covered, total))
}

// noCoverage is what a folder reports when there was nothing to measure: its
// fixtures loaded no PHP file of their own. A percentage there would be an
// answer to a question nobody asked, and a zero would drag the run's reading
// down for no finding.
const noCoverage = "-"

// percentOf rounds a ratio to whole percent.
func percentOf(covered, total int) int {
	if total == 0 {
		return 0
	}
	return int(float64(covered)/float64(total)*100 + 0.5)
}

// folderCoverage measures each group of fixtures against the PHP files those
// fixtures loaded. A file is charged to the folder whose fixtures loaded it,
// because a fixture's own directory is its include root: two folders including
// the same relative path are including their own copy of it.
func folderCoverage(groups []fixtureGroup) []folderCover {
	out := make([]folderCover, 0, len(groups))
	for _, group := range groups {
		rows := coverage.FileRows(reportBlocks(mergeCoverBlocks(group.Fixtures)), coverFiles(group.Fixtures))
		summary := folderCover{Dir: group.Dir, FilesTotal: len(rows), Files: rows}
		for _, r := range rows {
			// A file holding no runnable statement, a bag of declarations,
			// has nothing left to reach and counts as reached. This is the
			// same adjustment coverage.Row.Percent makes.
			if r.Covered > 0 || r.Total == 0 {
				summary.FilesCovered++
			}
			summary.LinesCovered += r.Covered
			summary.LinesTotal += r.Total
		}
		out = append(out, summary)
	}
	return out
}

// folderSummary is what the run prints for one folder when the fixture tables
// are not being printed. Coverage is present only when the run measured it.
type folderSummary struct {
	groupTotals
	cover *folderCover
}

// writeFolderTable prints one row per folder resolved from the arguments, which
// is what a run without -v answers with instead of a table per fixture. The
// columns follow what the run measured: with --cover the two coverage columns
// the issue asks for, without it the counts that exist.
func writeFolderTable(w io.Writer, rows []folderSummary, markdown, timings bool) {
	headers := []string{"Path", "Fixtures", "Passed", "Failed"}
	if timings {
		headers = append(headers, "Duration (ms)")
	}
	covered := len(rows) > 0 && rows[0].cover != nil
	if covered {
		headers = append(headers, "Files", "Lines")
	}

	values := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells := []string{
			row.Dir,
			strconv.Itoa(row.Total),
			strconv.Itoa(row.Passed),
			strconv.Itoa(row.Failed),
		}
		if timings {
			cells = append(cells, formatDuration(row.Duration))
		}
		if covered && row.cover != nil {
			cells = append(cells, row.cover.files(), row.cover.lines())
		}
		values = append(values, cells)
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = ansi.StringWidth(header)
	}
	for _, row := range values {
		for i, cell := range row {
			widths[i] = max(widths[i], ansi.StringWidth(cell))
		}
	}

	if markdown {
		writeMarkdownGrid(w, headers, values, widths)
		return
	}
	writeTerminalGrid(w, headers, values, widths)
}

// writeFolderFileReport prints the per-file coverage of each folder, which is
// what -v adds. It is the accounting the folder row summarises, one level down,
// and it is where an unvisited file is named rather than counted.
//
// A folder whose fixtures loaded no PHP file of their own is skipped: a heading
// over nothing reads as a folder that scored zero.
func writeFolderFileReport(w io.Writer, covers []folderCover) {
	for _, cover := range covers {
		if len(cover.Files) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n## coverage: %s\n\n", cover.Dir)
		tw := tabwriter.NewWriter(w, 0, 8, 1, ' ', 0)
		for _, r := range cover.Files {
			fmt.Fprintf(tw, "%s\t%d/%d lines covered\t%.1f%%\n", r.File, r.Covered, r.Total, r.Percent())
		}
		_ = tw.Flush()
		fmt.Fprintln(w)
	}
}

func writeMarkdownGrid(w io.Writer, headers []string, values [][]string, widths []int) {
	row := func(cells []string) {
		padded := make([]string, len(cells))
		for i, cell := range cells {
			padded[i] = cell + strings.Repeat(" ", max(0, widths[i]-ansi.StringWidth(cell)))
		}
		fmt.Fprintln(w, "| "+strings.Join(padded, " | ")+" |")
	}
	row(headers)
	separators := make([]string, len(widths))
	for i, width := range widths {
		separators[i] = strings.Repeat("-", width)
	}
	row(separators)
	for _, cells := range values {
		row(cells)
	}
	fmt.Fprintln(w)
}

func writeTerminalGrid(w io.Writer, headers []string, values [][]string, widths []int) {
	border := func(left, middle, right string) {
		segments := make([]string, len(widths))
		for i, width := range widths {
			segments[i] = strings.Repeat(table.BoxHorizontal, width+2)
		}
		fmt.Fprintln(w, table.ColorSeparator+left+strings.Join(segments, middle)+right+table.ColorReset)
	}
	row := func(cells []string, color string) {
		reset := ""
		if color != "" {
			reset = table.ColorReset
		}
		padded := make([]string, len(cells))
		for i, cell := range cells {
			padding := strings.Repeat(" ", max(0, widths[i]-ansi.StringWidth(cell)))
			padded[i] = " " + color + cell + padding + reset + " "
		}
		separator := table.ColorSeparator + table.BoxVertical + table.ColorReset
		fmt.Fprintln(w, separator+strings.Join(padded, separator)+separator)
	}

	border(table.BoxTopLeft, table.BoxTeeDown, table.BoxTopRight)
	row(headers, table.ColorHeader)
	border(table.BoxTeeRight, table.BoxCross, table.BoxTeeLeft)
	for _, cells := range values {
		row(cells, table.ColorWhite)
	}
	border(table.BoxBottomLeft, table.BoxTeeUp, table.BoxBottomRight)
	fmt.Fprintln(w)
}

// fixtureCoverage is one fixture's own statement coverage, which the fixture
// tables carry as a column under --cover. It is the coverage of the PHP the
// fixture loaded, not of the .phpt itself: the entrypoint runs by definition
// and would report every fixture at or near 100%.
func fixtureCoverage(fx *tests.Fixture) string {
	if fx == nil || fx.Coverage() == nil {
		return "-"
	}
	blocks := reportBlocks(fixtureCoverBlocks(fx))
	if len(blocks) == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", coverage.Percent(blocks))
}
