package table

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Align is how a column pads its cells.
type Align int

const (
	// Left pads a cell on the right, which is what a label wants.
	Left Align = iota
	// Right pads a cell on the left, which is what a number wants.
	Right
)

// Column is one column of a Table.
type Column struct {
	Title string
	Align Align
}

// Cell is one value and the color it is printed in. The color is dropped in
// markdown, so a caller sets it once rather than branching per output format.
type Cell struct {
	Text  string
	Color string
}

// Text returns an uncolored cell.
func Text(value string) Cell {
	return Cell{Text: value}
}

// Colored returns a cell printed in color on a terminal.
func Colored(color, value string) Cell {
	return Cell{Text: value, Color: color}
}

// Table buffers rows and renders them once the widest cell of every column is
// known. A check that collects its findings before reporting them gains
// nothing from streaming, and a buffered table sizes its columns from the data
// rather than guessing at them up front.
type Table struct {
	w        io.Writer
	markdown bool
	columns  []Column
	widths   []int
	rows     [][]Cell
}

// New returns a table writing to w. Markdown is what a redirected stdout and a
// report file get; a terminal gets the box-drawing table.
func New(w io.Writer, markdown bool, columns ...Column) *Table {
	t := &Table{
		w:        w,
		markdown: markdown,
		columns:  columns,
		widths:   make([]int, len(columns)),
	}
	for i, column := range columns {
		t.widths[i] = ansi.StringWidth(column.Title)
	}
	return t
}

// Row appends a row. Cells past the last column are dropped and missing ones
// are left empty, so a caller never has to pad a short row itself.
func (t *Table) Row(cells ...Cell) {
	row := make([]Cell, len(t.columns))
	for i := range row {
		if i < len(cells) {
			row[i] = Cell{Text: t.sanitize(cells[i].Text), Color: cells[i].Color}
		}
		if width := ansi.StringWidth(row[i].Text); width > t.widths[i] {
			t.widths[i] = width
		}
	}
	t.rows = append(t.rows, row)
}

// Flush writes the table. It is called once: the rows are kept so the columns
// can be sized, not so the table can be reprinted.
func (t *Table) Flush() {
	if t.markdown {
		t.flushMarkdown()
		return
	}
	t.flushTerminal()
}

// Summary writes a line below the table, in the header color on a terminal.
// The blank line before it is not decoration: markdown ends a table at the
// first line that is not a row, so a summary butted against one is read as
// part of it.
func (t *Table) Summary(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if !t.markdown {
		line = ColorHeader + line + ColorReset
	}
	fmt.Fprintf(t.w, "\n%s\n", line)
}

// sanitize keeps a value inside its cell: a row is one line, and in markdown a
// pipe would end the cell early.
func (t *Table) sanitize(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if t.markdown {
		value = strings.ReplaceAll(value, "|", `\|`)
	}
	return value
}

func (t *Table) flushTerminal() {
	t.writeBorder(BoxTopLeft, BoxTeeDown, BoxTopRight)

	headers := make([]Cell, len(t.columns))
	for i, column := range t.columns {
		headers[i] = Colored(ColorHeader, column.Title)
	}
	t.writeRow(headers)
	t.writeBorder(BoxTeeRight, BoxCross, BoxTeeLeft)

	for _, row := range t.rows {
		t.writeRow(row)
	}
	t.writeBorder(BoxBottomLeft, BoxTeeUp, BoxBottomRight)
}

func (t *Table) writeBorder(left, middle, right string) {
	segments := make([]string, len(t.widths))
	for i, width := range t.widths {
		segments[i] = strings.Repeat(BoxHorizontal, width+2)
	}
	fmt.Fprintln(t.w, ColorSeparator+left+strings.Join(segments, middle)+right+ColorReset)
}

func (t *Table) writeRow(row []Cell) {
	cells := make([]string, len(row))
	for i, cell := range row {
		value := cell.Text
		if cell.Color != "" && value != "" {
			value = cell.Color + value + ColorReset
		}
		cells[i] = " " + t.pad(i, value, cell.Text) + " "
	}
	separator := ColorSeparator + BoxVertical + ColorReset
	fmt.Fprintln(t.w, separator+strings.Join(cells, separator)+separator)
}

func (t *Table) flushMarkdown() {
	headers := make([]string, len(t.columns))
	separators := make([]string, len(t.columns))
	for i, column := range t.columns {
		headers[i] = t.pad(i, column.Title, column.Title)
		separators[i] = strings.Repeat("-", t.widths[i])
		if column.Align == Right {
			separators[i] = strings.Repeat("-", max(0, t.widths[i]-1)) + ":"
		}
	}
	fmt.Fprintln(t.w, "| "+strings.Join(headers, " | ")+" |")
	fmt.Fprintln(t.w, "| "+strings.Join(separators, " | ")+" |")

	for _, row := range t.rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = t.pad(i, cell.Text, cell.Text)
		}
		fmt.Fprintln(t.w, "| "+strings.Join(cells, " | ")+" |")
	}
}

// pad widens value to its column. The printed value carries color escapes that
// do not occupy a cell, so the width is measured on plain instead.
func (t *Table) pad(column int, value, plain string) string {
	padding := strings.Repeat(" ", max(0, t.widths[column]-ansi.StringWidth(plain)))
	if t.columns[column].Align == Right {
		return padding + value
	}
	return value + padding
}
