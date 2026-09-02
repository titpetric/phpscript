// Package helpdocs renders `phpscript --help`.
//
// The long help is a document rather than a flag dump: what the tool is, the
// commands, the flags every command shares, and per command its own flags and
// a table of worked examples. An agent reading it should not have to run nine
// commands to find out what the tenth accepts.
//
// The examples are markdown, one file per command, embedded from this
// directory. A terminal gets them through the same box-drawing table the rest
// of the tool prints; anything else gets the markdown back byte for byte, so
// `phpscript --help > help.md` is a document and not a screenshot of one.
package helpdocs

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/spf13/pflag"
	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/internal/table"
)

//go:embed *.md
var docs embed.FS

// FS returns the embedded example documents, named example.<command>.md.
func FS() fs.FS { return docs }

// Command is one entry of the long help: the name a user types, what it does,
// and the flags it defines beyond the shared set.
type Command struct {
	Name  string
	Title string
	Flags *cli.FlagSet
}

// Examples returns the rendered example document for one command, or the empty
// string when it has none. It is what a command's cli.Usage hook returns, so
// `phpscript test --help` carries the same table the long help does.
func Examples(name string, markdown bool) string {
	src, err := fs.ReadFile(docs, "example."+name+".md")
	if err != nil {
		return ""
	}
	var out bytes.Buffer
	if err := Render(&out, string(src), markdown); err != nil {
		return ""
	}
	return strings.TrimRight(out.String(), "\n")
}

// Write renders the whole help document: the usage line, the commands, the
// shared flags, then one section per command.
func Write(w io.Writer, name string, shared *cli.FlagSet, commands []Command, markdown bool) error {
	fmt.Fprintf(w, "# %s\n\n", name)
	fmt.Fprintf(w, "Usage: %s [flags] <command> [flags] [arguments]\n\n", name)
	fmt.Fprintf(w, "A command is required; with none, %s runs the php file named on the command line.\n\n", name)

	fmt.Fprintln(w, "## Commands")
	fmt.Fprintln(w)
	commandTable(w, commands, markdown)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Global flags")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Every command accepts these. A command reads the ones it has a use for.")
	fmt.Fprintln(w)
	flagTable(w, shared, markdown)

	for _, command := range commands {
		fmt.Fprintf(w, "\n## %s\n\n%s.\n\n", command.Name, command.Title)
		if command.Flags != nil && command.Flags.HasFlags() {
			flagTable(w, command.Flags, markdown)
			fmt.Fprintln(w)
		}
		if examples := Examples(command.Name, markdown); examples != "" {
			fmt.Fprintln(w, examples)
		}
	}
	return nil
}

// commandTable writes the command list.
func commandTable(w io.Writer, commands []Command, markdown bool) {
	t := table.New(w, markdown,
		table.Column{Title: "Command"},
		table.Column{Title: "What it does"},
	)
	for _, command := range commands {
		t.Row(table.Colored(table.ColorWhite, command.Name), table.Text(command.Title))
	}
	t.Flush()
}

// flagTable writes one flag set as a table. pflag prints its own defaults in a
// shape of its own; this is the shape the rest of the tool prints in.
func flagTable(w io.Writer, fs *cli.FlagSet, markdown bool) {
	t := table.New(w, markdown,
		table.Column{Title: "Flag"},
		table.Column{Title: "Default"},
		table.Column{Title: "What it does"},
	)
	fs.VisitAll(func(f *pflag.Flag) {
		name := "--" + f.Name
		if f.Shorthand != "" {
			name = "-" + f.Shorthand + ", " + name
		}
		value := f.DefValue
		if value == "" || value == "false" || value == "0" || value == "0s" {
			value = ""
		}
		t.Row(table.Colored(table.ColorWhite, name), table.Text(value), table.Text(f.Usage))
	})
	t.Flush()
}

// Render writes src to w, turning each markdown table into the table view the
// rest of the tool prints and passing everything else through unchanged.
//
// In markdown the source is written back as it was written. A round trip
// through the table renderer would re-pad every cell and rewrite alignment
// markers, which is a diff in a document nobody edited.
func Render(w io.Writer, src string, markdown bool) error {
	if markdown {
		_, err := io.WriteString(w, src)
		return err
	}

	// One trailing newline off the source, because every line is written with
	// one back. Without this a document ending the way documents end grows a
	// blank line each time it is rendered.
	lines := strings.Split(strings.TrimSuffix(src, "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		rows, next := tableAt(lines, i)
		if rows == nil {
			if _, err := fmt.Fprintln(w, lines[i]); err != nil {
				return err
			}
			continue
		}
		writeTable(w, rows)
		i = next - 1
	}
	return nil
}

// tableAt reads the markdown table starting at lines[i], or nil when there is
// none. A table is a row, a separator row, and the rows that follow.
func tableAt(lines []string, i int) ([][]string, int) {
	if !isRow(lines, i) || !isSeparator(lines, i+1) {
		return nil, i
	}
	rows := [][]string{cells(lines[i])}
	end := i + 2
	for ; end < len(lines) && isRow(lines, end); end++ {
		rows = append(rows, cells(lines[end]))
	}
	return rows, end
}

func isRow(lines []string, i int) bool {
	return i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|")
}

// isSeparator reports whether the line is a table's header rule: cells holding
// nothing but dashes and the alignment colons.
func isSeparator(lines []string, i int) bool {
	if !isRow(lines, i) {
		return false
	}
	for _, cell := range cells(lines[i]) {
		if cell == "" || strings.Trim(cell, "-:") != "" {
			return false
		}
	}
	return true
}

// cells splits one markdown row into its values.
func cells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
	out := strings.Split(line, "|")
	for i, cell := range out {
		out[i] = strings.TrimSpace(cell)
	}
	return out
}

// writeTable renders parsed rows, the first of which is the header.
func writeTable(w io.Writer, rows [][]string) {
	header := rows[0]
	columns := make([]table.Column, len(header))
	for i, title := range header {
		columns[i] = table.Column{Title: title}
	}
	t := table.New(w, false, columns...)
	for _, row := range rows[1:] {
		out := make([]table.Cell, len(row))
		for i, value := range row {
			out[i] = table.Text(value)
		}
		if len(out) > 0 {
			out[0] = table.Colored(table.ColorWhite, row[0])
		}
		t.Row(out...)
	}
	t.Flush()
}
