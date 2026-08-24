package table

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTableRendersMarkdown(t *testing.T) {
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

	// Every row is padded to one width, which is what keeps the box square.
	lines := strings.Split(strings.TrimSpace(ansi.Strip(got)), "\n")
	for i, line := range lines[:5] {
		if ansi.StringWidth(line) != ansi.StringWidth(lines[0]) {
			t.Errorf("line %d has width %d, want %d:\n%s", i, ansi.StringWidth(line), ansi.StringWidth(lines[0]), got)
		}
	}
}

// TestTableRowKeepsCellsIntact covers the two ways a value escapes its cell: a
// newline would end the row, and in markdown a pipe would end the cell.
func TestTableRowKeepsCellsIntact(t *testing.T) {
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
// row has nothing to say in the last one.
func TestTableRowPadsShortRows(t *testing.T) {
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
