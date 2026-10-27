package table

import (
	"bytes"
	"strings"
	"testing"
)

func TestTableRendersMarkdown(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	table := newSample(&buf, true)
	table.Flush()
	table.Summary("Passing %d", 1)

	want := strings.Join([]string{
		"| Status | File     | Line |",
		"| ------ | -------- | ---: |",
		"| PASS   | a.php    |      |",
		"| FAIL   | long.php |   12 |",
		"",
		"Passing 1",
		"",
	}, "\n")
	if got := buf.String(); got != want {
		t.Errorf("markdown table =\n%s\nwant\n%s", got, want)
	}
}

func TestTableRendersAnsi(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	table := newSample(&buf, false)
	table.Flush()
	table.Summary("Passing %d", 1)

	got := buf.String()
	for _, want := range []string{
		ColorSeparator + BoxTopLeft,
		ColorHeader + "Status",
		ColorGreen + "PASS",
		ColorRed + "FAIL",
		ColorHeader + "Passing 1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ansi table does not contain %q:\n%q", want, got)
		}
	}

	// Every row carries one width, which keeps the box square.
	lines := strings.Split(strings.TrimSpace(Strip(got)), "\n")
	for i, line := range lines[:5] {
		if Width(line) != Width(lines[0]) {
			t.Errorf("line %d has width %d, want %d:\n%s", i, Width(line), Width(lines[0]), got)
		}
	}
}

// TestTableRowKeepsCellsIntact covers the two ways a value escapes its cell: a
// newline would end the row, and in markdown a pipe would end the cell.
func TestTableRowKeepsCellsIntact(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	table := New(&buf, true, Column{Title: "Message"})
	table.Row(Text("got: a|b\nwant: c"))
	table.Flush()

	want := `| got: a\|b want: c |`
	if !strings.Contains(buf.String(), want) {
		t.Errorf("row = %q, want it to contain %q", buf.String(), want)
	}
}

// TestTableRowPadsShortRows keeps a caller from having to count columns when a
// row leaves the last one empty.
func TestTableRowPadsShortRows(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	table := New(&buf, true, Column{Title: "A"}, Column{Title: "B"})
	table.Row(Text("one"))
	table.Row(Text("one"), Text("two"), Text("dropped"))
	table.Flush()

	got := buf.String()
	if !strings.Contains(got, "| one |     |") {
		t.Errorf("short row is not padded:\n%s", got)
	}
	if strings.Contains(got, "dropped") {
		t.Errorf("cell past the last column was printed:\n%s", got)
	}
}

// TestAlign covers which side a column pads on. A label reads from the left
// and a number reads from the right and the two alignments put the padding on
// opposite sides of the cell. In markdown the alignment rides on the
// separator row, where a trailing colon is what a renderer reads as right.
func TestAlign(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		align    Align
		wantCell string
		wantRule string
	}{
		{name: "left", align: Left, wantCell: "| x      |", wantRule: "| ------ |"},
		{name: "right", align: Right, wantCell: "|      x |", wantRule: "| -----: |"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			table := New(&buf, true, Column{Title: "Header", Align: tt.align})
			table.Row(Text("x"))
			table.Flush()

			got := buf.String()
			if !strings.Contains(got, tt.wantCell) {
				t.Errorf("cell =\n%s\nwant a line containing %q", got, tt.wantCell)
			}
			if !strings.Contains(got, tt.wantRule) {
				t.Errorf("separator =\n%s\nwant a line containing %q", got, tt.wantRule)
			}
		})
	}

	// Left is the zero value and a Column written without an Align reads as a
	// label.
	if Left != 0 {
		t.Errorf("Left = %d, want the zero value so an unset Align is left", Left)
	}
	if Right == Left {
		t.Error("Right and Left are the same value")
	}
}

// TestCell covers the one value a cell carries and the color a terminal prints
// it
// in. A caller sets the color once; the terminal table wraps the text in it
// and markdown drops it and the same rows render in both formats with no
// branch at the call site.
func TestCell(t *testing.T) {
	t.Parallel()
	if got := Text("plain"); got != (Cell{Text: "plain"}) {
		t.Errorf("Text(plain) = %+v, want an uncolored cell", got)
	}
	if got := Colored(ColorRed, "FAIL"); got != (Cell{Text: "FAIL", Color: ColorRed}) {
		t.Errorf("Colored(red, FAIL) = %+v", got)
	}

	var terminal bytes.Buffer
	ansiTable := New(&terminal, false, Column{Title: "Status"})
	ansiTable.Row(Colored(ColorRed, "FAIL"))
	ansiTable.Flush()
	if !strings.Contains(terminal.String(), ColorRed+"FAIL"+ColorReset) {
		t.Errorf("ansi row does not print the cell in its color:\n%q", terminal.String())
	}

	var markdown bytes.Buffer
	mdTable := New(&markdown, true, Column{Title: "Status"})
	mdTable.Row(Colored(ColorRed, "FAIL"))
	mdTable.Flush()
	if strings.Contains(markdown.String(), "\033") {
		t.Errorf("markdown row carries an escape sequence:\n%q", markdown.String())
	}
	if !strings.Contains(markdown.String(), "| FAIL   |") {
		t.Errorf("markdown row =\n%s\nwant the text padded to the header width", markdown.String())
	}

	// An empty cell is never colored: a color around nothing would print an
	// escape pair into a blank column.
	var empty bytes.Buffer
	emptyTable := New(&empty, false, Column{Title: "Status"})
	emptyTable.Row(Colored(ColorRed, ""))
	emptyTable.Flush()
	if strings.Contains(empty.String(), ColorRed) {
		t.Errorf("an empty cell was colored:\n%q", empty.String())
	}
}

func newSample(buf *bytes.Buffer, markdown bool) *Table {
	table := New(buf, markdown,
		Column{Title: "Status"},
		Column{Title: "File"},
		Column{Title: "Line", Align: Right},
	)
	table.Row(Colored(ColorGreen, "PASS"), Colored(ColorAmber, "a.php"), Text(""))
	table.Row(Colored(ColorRed, "FAIL"), Colored(ColorAmber, "long.php"), Text("12"))
	return table
}
