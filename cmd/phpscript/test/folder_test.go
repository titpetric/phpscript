package test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/tests"
)

// TestRatio covers the form both coverage columns take. The header says what
// the column counts and the cell carries the counts and the percent they come
// to, and the percent rounds to whole: 1 of 2 reads 50%, where truncation
// would read 49%.
func TestRatio(t *testing.T) {
	t.Parallel()
	for want, counts := range map[string][2]int{
		"4/50 (8%)":       {4, 50},
		"1532/8000 (19%)": {1532, 8000},
		"1/2 (50%)":       {1, 2},
		"2/3 (67%)":       {2, 3},
		"1/3 (33%)":       {1, 3},
		"0/5 (0%)":        {0, 5},
		"5/5 (100%)":      {5, 5},
	} {
		if got := ratio(counts[0], counts[1]); got != want {
			t.Errorf("ratio(%d, %d) = %q, want %q", counts[0], counts[1], got, want)
		}
	}
}

// TestPercentOf covers the rounding, and the nothing-to-measure case: a zero
// total is no percent at all. Dividing by it would panic, and answering 100%
// for a folder with no files would read as a measurement.
func TestPercentOf(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		covered, total, want int
	}{
		{0, 0, 0},
		{4, 0, 0},
		{0, 10, 0},
		{1, 2, 50},
		{2, 3, 67},
		{1, 3, 33},
		{10, 10, 100},
	} {
		if got := percentOf(test.covered, test.total); got != test.want {
			t.Errorf("percentOf(%d, %d) = %d, want %d", test.covered, test.total, got, test.want)
		}
	}
}

// TestFolderCoverColumns covers what a folder with nothing to measure prints.
// Its fixtures loaded no PHP file of their own and both columns answer with the
// dash: a zero percent there would drag the run's reading down for no finding.
func TestFolderCoverColumns(t *testing.T) {
	t.Parallel()
	empty := folderCover{Dir: "arrays"}
	if got := empty.files(); got != noCoverage {
		t.Errorf("files() = %q with nothing measured, want %q", got, noCoverage)
	}
	if got := empty.lines(); got != noCoverage {
		t.Errorf("lines() = %q with nothing measured, want %q", got, noCoverage)
	}

	measured := folderCover{
		Dir:          "arrays",
		FilesCovered: 4,
		FilesTotal:   50,
		LinesCovered: 1532,
		LinesTotal:   8000,
	}
	if got, want := measured.files(), "4/50 (8%)"; got != want {
		t.Errorf("files() = %q, want %q", got, want)
	}
	if got, want := measured.lines(), "1532/8000 (19%)"; got != want {
		t.Errorf("lines() = %q, want %q", got, want)
	}
}

// TestFolderCoverageCountsADeclarationFileAsReached covers the one exception
// the file count makes: a file holding no runnable statement, a bag of
// declarations, has nothing left to reach and counts as reached. Without it
// the count would charge a suite including a file of class declarations for
// coverage it cannot reach.
func TestFolderCoverageCountsADeclarationFileAsReached(t *testing.T) {
	t.Parallel()
	rows := []coverage.Row{
		{File: "declarations.php", Covered: 0, Total: 0},
		{File: "reached.php", Covered: 3, Total: 4},
		{File: "untouched.php", Covered: 0, Total: 6},
	}

	summary := folderCover{Dir: "arrays", FilesTotal: len(rows), Files: rows}
	for _, row := range rows {
		if row.Covered > 0 || row.Total == 0 {
			summary.FilesCovered++
		}
		summary.LinesCovered += row.Covered
		summary.LinesTotal += row.Total
	}

	if summary.FilesCovered != 2 {
		t.Errorf("FilesCovered = %d, want the reached file and the declaration one", summary.FilesCovered)
	}
	if got, want := summary.files(), "2/3 (67%)"; got != want {
		t.Errorf("files() = %q, want %q", got, want)
	}
	if got, want := summary.lines(), "3/10 (30%)"; got != want {
		t.Errorf("lines() = %q, want %q", got, want)
	}
}

// TestFolderCoverageGroupsWithNothingLoaded covers the measurement over groups
// whose fixtures loaded no file: one row per group, each reporting nothing and
// the table has a line for every folder the arguments resolved to.
func TestFolderCoverageGroupsWithNothingLoaded(t *testing.T) {
	t.Parallel()
	groups := []fixtureGroup{
		{Dir: "arrays", Fixtures: []*tests.Fixture{{Name: "a"}}},
		{Dir: "strings", Fixtures: []*tests.Fixture{{Name: "s"}}},
	}

	covers := folderCoverage(groups)
	if len(covers) != len(groups) {
		t.Fatalf("covers = %d, want one per group", len(covers))
	}
	for i, cover := range covers {
		if cover.Dir != groups[i].Dir {
			t.Errorf("cover %d is for %q, want %q", i, cover.Dir, groups[i].Dir)
		}
		if cover.files() != noCoverage || cover.lines() != noCoverage {
			t.Errorf("%s measured %s files and %s lines with nothing loaded", cover.Dir, cover.files(), cover.lines())
		}
	}
}

// TestFolderTableSplitsEngineDurations covers the folder summary of a matrix
// run with cost columns: one duration column per runner after the wall-clock
// one. Without engines the single column stays, as a plain run and
// the checked-in report print.
func TestFolderTableSplitsEngineDurations(t *testing.T) {
	t.Parallel()
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

// TestWriteFolderTableWithCoverage covers the two columns --cover adds, and
// that a folder with nothing measured keeps its row: the run reports every
// folder the arguments resolved to, measured or not.
func TestWriteFolderTableWithCoverage(t *testing.T) {
	t.Parallel()
	measured := folderCover{Dir: "arrays", FilesCovered: 4, FilesTotal: 50, LinesCovered: 1532, LinesTotal: 8000}
	rows := []folderSummary{
		{groupTotals: groupTotals{Dir: "arrays", Passed: 1, Total: 1}, cover: &measured},
		{groupTotals: groupTotals{Dir: "strings", Passed: 2, Total: 2}, cover: &folderCover{Dir: "strings"}},
	}

	var buf bytes.Buffer
	writeFolderTable(&buf, rows, true, false)
	got := buf.String()

	for _, want := range []string{"Files", "Lines", "4/50 (8%)", "1532/8000 (19%)", "strings"} {
		if !strings.Contains(got, want) {
			t.Errorf("folder table is missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, noCoverage) == 0 {
		t.Errorf("the unmeasured folder reports no dash:\n%s", got)
	}
}
