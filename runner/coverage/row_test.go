package coverage_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner/coverage"
)

// TestRow_Percent covers the adjusted percentage: a symbol with no runnable
// statement has nothing left uncovered, so 0/0 reads as covered rather than as
// a zero dragging every average down.
func TestRow_Percent(t *testing.T) {
	for _, tc := range []struct {
		row  coverage.Row
		want float64
	}{
		{row: coverage.Row{Covered: 3, Total: 4}, want: 75},
		{row: coverage.Row{Covered: 0, Total: 2}, want: 0},
		{row: coverage.Row{}, want: 100},
	} {
		if got := tc.row.Percent(); got != tc.want {
			t.Errorf("Row%+v.Percent() = %v, want %v", tc.row, got, tc.want)
		}
	}
}

// TestFileRows covers the per-file report, including a registered file holding
// nothing runnable.
func TestFileRows(t *testing.T) {
	blocks := coverage.Columns(profileBlocks, testSource)
	rows := coverage.FileRows(blocks, []string{"app.php", "lib.php", "empty.php"})
	got := map[string]float64{}
	for _, row := range rows {
		got[row.File] = row.Percent()
	}
	for file, want := range map[string]float64{"app.php": 50, "lib.php": 100, "empty.php": 100} {
		if got[file] != want {
			t.Errorf("%s = %v%%, want %v%%", file, got[file], want)
		}
	}
	if rows[0].Name != "app.php" {
		t.Errorf("row name = %q, want the file's base name", rows[0].Name)
	}
}

// TestFuncRows covers the per-function report: a block is charged to the
// innermost declaration containing it, top-level code to {main}, and an
// uncalled function is the zero row the report exists to show.
func TestFuncRows(t *testing.T) {
	blocks := coverage.Columns(profileBlocks, testSource)
	funcs := []coverage.FuncSpan{
		{File: "lib.php", Name: "f", StartLine: 2, EndLine: 2},
		{File: "lib.php", Name: "g", StartLine: 4, EndLine: 6},
	}
	rows := coverage.FuncRows(blocks, funcs, []string{"app.php", "lib.php"})

	got := map[string]coverage.Row{}
	for _, row := range rows {
		got[row.Name] = row
	}
	if row, ok := got["f"]; !ok || row.Percent() != 100 {
		t.Errorf("f = %+v, want 100%%", row)
	}
	// g holds no counted block, so its adjusted 0/0 reads as covered and
	// contributes nothing to the total.
	if row, ok := got["g"]; !ok || row.Total != 0 || row.Percent() != 100 {
		t.Errorf("g = %+v, want an empty row", row)
	}
	if row, ok := got["{main}"]; !ok || row.File != "app.php" || row.Percent() != 50 {
		t.Errorf("{main} = %+v, want app.php at 50%%", row)
	}
}

// TestWriteReport pins the shape `summary coverfunc` parses: three
// whitespace-separated columns and a total row it skips by name.
func TestWriteReport(t *testing.T) {
	var out strings.Builder
	rows := coverage.FileRows(coverage.Columns(profileBlocks, testSource), []string{"app.php", "lib.php"})
	if err := coverage.WriteReport(&out, rows); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("report =\n%s\nwant two rows and a total", out.String())
	}
	for _, line := range lines {
		if fields := strings.Fields(line); len(fields) != 3 {
			t.Errorf("line %q splits into %d fields, want 3", line, len(fields))
		}
	}
	if !strings.HasPrefix(lines[0], "app.php:1:") {
		t.Errorf("first row = %q, want a file:line: location", lines[0])
	}
	if !strings.HasPrefix(lines[2], "total:") || !strings.HasSuffix(lines[2], "75.0%") {
		t.Errorf("total row = %q, want 75.0%%", lines[2])
	}
}

// TestSortRows pins the order a report is written in.
func TestSortRows(t *testing.T) {
	rows := []coverage.Row{
		{File: "b.php", Line: 1, Name: "x"},
		{File: "a.php", Line: 9, Name: "y"},
		{File: "a.php", Line: 2, Name: "z"},
		{File: "a.php", Line: 2, Name: "a"},
	}
	coverage.SortRows(rows)
	var got []string
	for _, row := range rows {
		got = append(got, row.File+":"+row.Name)
	}
	want := "a.php:a a.php:z a.php:y b.php:x"
	if strings.Join(got, " ") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
}

// TestSortProfile pins the order a profile is written in.
func TestSortProfile(t *testing.T) {
	blocks := []coverage.ProfileBlock{
		{File: "b.php", StartLine: 1, EndLine: 1},
		{File: "a.php", StartLine: 4, EndLine: 4},
		{File: "a.php", StartLine: 2, EndLine: 9},
		{File: "a.php", StartLine: 2, EndLine: 3},
	}
	coverage.SortProfile(blocks)
	var got []string
	for _, block := range blocks {
		got = append(got, block.File)
	}
	if got[0] != "a.php" || got[3] != "b.php" {
		t.Errorf("order = %v, want a.php first", got)
	}
	if blocks[0].EndLine != 3 || blocks[1].EndLine != 9 {
		t.Errorf("blocks sharing a start line = %+v, want the shorter one first", blocks[:2])
	}
}
