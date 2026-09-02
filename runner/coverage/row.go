package coverage

import (
	"fmt"
	"io"
	"path"
	"sort"
	"text/tabwriter"
)

// Row is one line of a func or file report: the location column go tool cover
// -func prints, the symbol charged, and the statements counted against it.
type Row struct {
	File    string
	Line    int
	Name    string
	Covered int
	Total   int
}

// Percent is the row's statement-weighted coverage, adjusted for the empty
// case: a symbol with no runnable statement has nothing left uncovered, so 0/0
// reads as covered rather than as a zero dragging every average down.
func (r Row) Percent() float64 {
	if r.Total == 0 {
		return 100
	}
	return float64(r.Covered) / float64(r.Total) * 100
}

// FuncRows charges each profile block to the innermost declaration span
// containing its start line. A block inside no declaration is the file's
// top-level code, reported as {main} - the name PHP gives that scope - at the
// first such line. files names every registered file, so one holding nothing
// runnable still reports a row.
func FuncRows(blocks []ProfileBlock, funcs []FuncSpan, files []string) []Row {
	type key struct {
		file string
		line int
		name string
	}
	rows := map[key]*Row{}
	row := func(file string, line int, name string) *Row {
		k := key{file: file, line: line, name: name}
		r, ok := rows[k]
		if !ok {
			r = &Row{File: file, Line: line, Name: name}
			rows[k] = r
		}
		return r
	}
	for _, fn := range funcs {
		row(fn.File, fn.StartLine, fn.Name)
	}
	for _, b := range blocks {
		var at *FuncSpan
		for i := range funcs {
			fn := &funcs[i]
			if fn.File != b.File || b.StartLine < fn.StartLine || b.StartLine > fn.EndLine {
				continue
			}
			if at == nil || fn.EndLine-fn.StartLine < at.EndLine-at.StartLine {
				at = fn
			}
		}
		var r *Row
		if at != nil {
			r = row(at.File, at.StartLine, at.Name)
		} else {
			r = row(b.File, 1, "{main}")
		}
		r.Total += b.NumStmt
		if b.Count > 0 {
			r.Covered += b.NumStmt
		}
	}
	// A registered file none of whose declarations or top-level lines charged
	// anything holds nothing runnable; it reports as one adjusted {main} row
	// rather than disappearing.
	charged := map[string]bool{}
	for k := range rows {
		charged[k.file] = true
	}
	for _, file := range files {
		if !charged[file] {
			row(file, 1, "{main}")
		}
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	SortRows(out)
	return out
}

// FileRows reports one row per registered source file. A file with no runnable
// statement stays in the report at its adjusted percentage.
func FileRows(blocks []ProfileBlock, files []string) []Row {
	rows := map[string]*Row{}
	row := func(file string) *Row {
		r, ok := rows[file]
		if !ok {
			r = &Row{File: file, Line: 1, Name: path.Base(file)}
			rows[file] = r
		}
		return r
	}
	for _, file := range files {
		row(file)
	}
	for _, b := range blocks {
		r := row(b.File)
		r.Total += b.NumStmt
		if b.Count > 0 {
			r.Covered += b.NumStmt
		}
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	SortRows(out)
	return out
}

// SortRows orders report rows by file, then position, then symbol.
func SortRows(rows []Row) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].File != rows[j].File {
			return rows[i].File < rows[j].File
		}
		if rows[i].Line != rows[j].Line {
			return rows[i].Line < rows[j].Line
		}
		return rows[i].Name < rows[j].Name
	})
}

// WriteReport prints rows the way go tool cover -func does: location, symbol
// and percent columns, and a statement-weighted total line that downstream
// consumers recognize by name and skip.
func WriteReport(w io.Writer, rows []Row) error {
	tw := tabwriter.NewWriter(w, 0, 8, 1, '\t', 0)
	var covered, total int
	for _, r := range rows {
		covered += r.Covered
		total += r.Total
		fmt.Fprintf(tw, "%s:%d:\t%s\t%.1f%%\n", r.File, r.Line, r.Name, r.Percent())
	}
	pct := 0.0
	if total > 0 {
		pct = float64(covered) / float64(total) * 100
	}
	fmt.Fprintf(tw, "total:\t(statements)\t%.1f%%\n", pct)
	return tw.Flush()
}
