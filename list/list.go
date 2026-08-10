// Package list builds a markdown inventory of PHP routes, files and classes.
package list

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/route"
)

// Row is one markdown table row.
type Row struct {
	Route    string // e.g. "GET /users/{id}", or empty
	Filename string // slash-separated path relative to the working directory
	Classes  string // comma-separated FQNs, or empty
}

// Paths scans path arguments and returns inventory rows.
func Paths(paths []string) ([]Row, error) {
	files, err := ExpandFiles(paths)
	if err != nil {
		return nil, err
	}
	var rows []Row
	for _, file := range files {
		fileRows, err := File(file)
		if err != nil {
			return nil, err
		}
		rows = append(rows, fileRows...)
	}
	return rows, nil
}

// File returns inventory rows for one PHP source file.
func File(path string) ([]Row, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rel := filepath.ToSlash(path)
	classes, err := classNames(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	classCol := strings.Join(classes, ", ")
	anns := route.Annotations(b)
	if len(anns) == 0 {
		return []Row{{
			Filename: rel,
			Classes:  classCol,
		}}, nil
	}
	rows := make([]Row, 0, len(anns))
	for _, a := range anns {
		rows = append(rows, Row{
			Route:    a.Method + " " + a.Path,
			Filename: rel,
			Classes:  classCol,
		})
	}
	return rows, nil
}

func classNames(src string) ([]string, error) {
	prog, err := parser.Parse(src)
	if err != nil {
		// Unsupported syntax (extends, etc.) still lists via a token scan.
		return classNamesFromTokens(src), nil
	}
	var names []string
	collectClasses(prog.Stmts, &names)
	if len(names) == 0 {
		return classNamesFromTokens(src), nil
	}
	return names, nil
}

func classNamesFromTokens(src string) []string {
	ns := ""
	var names []string
	toks := tokenize(src)
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.id == parser.T_STRING && strings.EqualFold(t.text, "namespace") {
			if name, ok := readQualifiedName(toks, i+1); ok {
				ns = name
			}
			continue
		}
		if t.id == parser.T_CLASS {
			j := nextNonWS(toks, i+1)
			if j < len(toks) && toks[j].id == parser.T_STRING {
				name := toks[j].text
				if ns != "" {
					name = ns + "\\" + name
				}
				names = append(names, name)
			}
		}
	}
	return names
}

type scanTok struct {
	id   int
	text string
}

func tokenize(src string) []scanTok {
	var out []scanTok
	parser.TokenGetAll(src).Range(func(_, val any) bool {
		if a, ok := val.(*model.Array); ok {
			id, _ := a.Get(int64(0))
			text, _ := a.Get(int64(1))
			out = append(out, scanTok{id: int(id.(int64)), text: text.(string)})
			return true
		}
		if s, ok := val.(string); ok {
			out = append(out, scanTok{text: s})
		}
		return true
	})
	return out
}

func nextNonWS(toks []scanTok, i int) int {
	for i < len(toks) {
		if toks[i].id != parser.T_WHITESPACE && toks[i].id != parser.T_COMMENT {
			return i
		}
		i++
	}
	return i
}

func readQualifiedName(toks []scanTok, i int) (string, bool) {
	i = nextNonWS(toks, i)
	if i >= len(toks) || toks[i].id != parser.T_STRING {
		return "", false
	}
	parts := []string{toks[i].text}
	i++
	for {
		i = nextNonWS(toks, i)
		if i >= len(toks) || toks[i].text != "\\" {
			break
		}
		i = nextNonWS(toks, i+1)
		if i >= len(toks) || toks[i].id != parser.T_STRING {
			break
		}
		parts = append(parts, toks[i].text)
		i++
	}
	return strings.Join(parts, "\\"), true
}

func collectClasses(stmts []model.Stmt, out *[]string) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *model.ClassDecl:
			*out = append(*out, n.Name)
		case *model.If:
			collectClasses(n.Then, out)
			collectClasses(n.Else, out)
		case *model.For:
			collectClasses(n.Body, out)
		case *model.Foreach:
			collectClasses(n.Body, out)
		case *model.Try:
			collectClasses(n.Body, out)
			for _, c := range n.Catches {
				collectClasses(c.Body, out)
			}
			collectClasses(n.Finally, out)
		case *model.Switch:
			for _, c := range n.Cases {
				collectClasses(c.Body, out)
			}
			collectClasses(n.Default, out)
		case *model.FuncDecl:
			collectClasses(n.Body, out)
		}
	}
}

// Markdown renders rows as an indented, padded GitHub-flavoured markdown table.
func Markdown(rows []Row) string {
	headings := [3]string{"Route", "Filename", "Classes"}
	widths := [3]int{len(headings[0]), len(headings[1]), len(headings[2])}
	for _, r := range rows {
		values := [3]string{cell(r.Route), filenameCell(r.Filename), cell(r.Classes)}
		for i, value := range values {
			widths[i] = max(widths[i], utf8.RuneCountInString(value))
		}
	}

	var b strings.Builder
	writeMarkdownRow(&b, headings, widths)
	b.WriteString("  |")
	for _, width := range widths {
		fmt.Fprintf(&b, "%s|", strings.Repeat("-", width+2))
	}
	b.WriteByte('\n')
	for _, r := range rows {
		writeMarkdownRow(&b, [3]string{cell(r.Route), filenameCell(r.Filename), cell(r.Classes)}, widths)
	}
	return b.String()
}

func writeMarkdownRow(b *strings.Builder, values [3]string, widths [3]int) {
	b.WriteString("  |")
	for i, value := range values {
		fmt.Fprintf(b, " %s%s |", value, strings.Repeat(" ", widths[i]-utf8.RuneCountInString(value)))
	}
	b.WriteByte('\n')
}

func cell(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}

func filenameCell(path string) string {
	if path == "" {
		return "<none>"
	}
	return fmt.Sprintf("[%s](./%s)", path, path)
}
