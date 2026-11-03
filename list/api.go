package list

import (
	"os"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
)

// MethodRow is one class or interface member of the API listing. Member is the
// method name, Signature carries the declarations the reader needs to call it,
// and Doc is the docblock above the declaration with its comment markers
// stripped, on one line.
type MethodRow struct {
	Class     string `json:"class"`
	Member    string `json:"member"`
	Signature string `json:"signature"`
	Doc       string `json:"doc"`
}

// API scans path arguments and returns one row per method of every class and
// interface the files declare. The API includes interfaces because an `implements`
// contract is part of the API a reader needs.
func API(paths []string) ([]MethodRow, error) {
	files, err := ExpandFiles(paths)
	if err != nil {
		return nil, err
	}
	var rows []MethodRow
	for _, file := range files {
		fileRows, err := APIFile(file)
		if err != nil {
			return nil, err
		}
		rows = append(rows, fileRows...)
	}
	return rows, nil
}

// APIFile returns the API rows for one PHP source file. A file the parser
// refuses contributes no rows: the token scan that the inventory falls back to
// can name a class but cannot recover its methods, parameters or docblocks.
func APIFile(path string) ([]MethodRow, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := string(b)
	prog, err := parser.Parse(src)
	if err != nil {
		return []MethodRow{}, nil
	}
	lines := strings.Split(src, "\n")
	var rows []MethodRow
	collectAPIMethods(prog.Stmts, prog.SourceSpans, lines, &rows)
	return rows, nil
}

// collectAPIMethods walks the statement tree and appends the methods of every
// class and interface it finds. It keeps the declaration order inside a file;
// the caller sorts, because a template needs the source order.
func collectAPIMethods(stmts []model.Stmt, spans map[model.Stmt]model.SourceSpan, lines []string, out *[]MethodRow) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *model.ClassDecl:
			appendMethods(n.Name, n.Methods, spans, lines, out)
		case *model.InterfaceDecl:
			appendMethods(n.Name, n.Methods, spans, lines, out)
		case *model.If:
			collectAPIMethods(n.Then, spans, lines, out)
			collectAPIMethods(n.Else, spans, lines, out)
		case *model.For:
			collectAPIMethods(n.Body, spans, lines, out)
		case *model.DoWhile:
			collectAPIMethods(n.Body, spans, lines, out)
		case *model.Foreach:
			collectAPIMethods(n.Body, spans, lines, out)
		case *model.Try:
			collectAPIMethods(n.Body, spans, lines, out)
			for _, c := range n.Catches {
				collectAPIMethods(c.Body, spans, lines, out)
			}
			collectAPIMethods(n.Finally, spans, lines, out)
		case *model.Switch:
			for _, c := range n.Cases {
				collectAPIMethods(c.Body, spans, lines, out)
			}
			collectAPIMethods(n.Default, spans, lines, out)
		case *model.FuncDecl:
			collectAPIMethods(n.Body, spans, lines, out)
		}
	}
}

func appendMethods(class string, methods []*model.FuncDecl, spans map[model.Stmt]model.SourceSpan, lines []string, out *[]MethodRow) {
	for _, m := range methods {
		doc := ""
		if span, ok := spans[m]; ok {
			doc = docAbove(lines, span.Start)
		}
		*out = append(*out, MethodRow{
			Class:     class,
			Member:    m.Name,
			Signature: methodSignature(m),
			Doc:       doc,
		})
	}
}

// methodSignature renders `name(params): type`, with `static` ahead of the name
// when the declaration is static. It omits visibility: the inventory is an
// API document, and a private method is an implementation detail a reader does
// not need the modifier for.
func methodSignature(m *model.FuncDecl) string {
	var b strings.Builder
	if m.Static {
		b.WriteString("static ")
	}
	b.WriteString(m.Name)
	b.WriteString("(")
	for i, p := range m.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(paramSignature(p))
	}
	b.WriteString(")")
	if m.ReturnType != "" {
		b.WriteString(": ")
		b.WriteString(m.ReturnType)
	}
	return b.String()
}

func paramSignature(p model.Param) string {
	var b strings.Builder
	if p.Modifiers != "" {
		b.WriteString(p.Modifiers)
		b.WriteByte(' ')
	}
	if p.Type != "" {
		b.WriteString(p.Type)
		b.WriteByte(' ')
	}
	if p.ByRef {
		b.WriteByte('&')
	}
	if p.Variadic {
		b.WriteString("...")
	}
	b.WriteByte('$')
	b.WriteString(p.Name)
	if p.Default != nil {
		if def := defaultSignature(p.Default); def != "" {
			b.WriteString(" = ")
			b.WriteString(def)
		}
	}
	return b.String()
}

// defaultSignature renders a parameter default. The shapes a default takes in
// practice are literals, constants, arrays and small arithmetic expressions;
// a shape outside them contributes nothing, never an invented spelling.
func defaultSignature(e model.Expr) string {
	switch n := model.UnwrapParenthesized(e).(type) {
	case *model.Lit:
		switch v := n.Value.(type) {
		case nil:
			return "null"
		case bool:
			if v {
				return "true"
			}
			return "false"
		case string:
			return "'" + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), "'", `\'`) + "'"
		case int:
			return strconv.Itoa(v)
		case int64:
			return strconv.FormatInt(v, 10)
		case float64:
			return strconv.FormatFloat(v, 'g', -1, 64)
		}
		return ""
	case *model.Var:
		if n.Const {
			return n.Name
		}
		return "$" + n.Name
	case *model.ClassConst:
		return n.Class + "::" + n.Name
	case *model.ArrayLit:
		parts := make([]string, 0, len(n.Items))
		for _, it := range n.Items {
			val := defaultSignature(it.Val)
			if val == "" {
				return ""
			}
			if it.Key != nil {
				key := defaultSignature(it.Key)
				if key == "" {
					return ""
				}
				parts = append(parts, key+" => "+val)
			} else {
				parts = append(parts, val)
			}
		}
		return "array(" + strings.Join(parts, ", ") + ")"
	case *model.Unary:
		val := defaultSignature(n.X)
		if val == "" {
			return ""
		}
		return n.Op + val
	}
	return ""
}

// docAbove returns the docblock written immediately above a declaration
// starting at line (1-based), with the comment markers removed and the lines
// joined by spaces. A blank line between the docblock and the declaration
// ends the search, as it does for the tools that render Go documentation.
func docAbove(lines []string, line int) string {
	// line is 1-based and lines is 0-based: the declaration is lines[line-1]
	// and lines[line-2] holds the line above.
	i := line - 2
	if i < 0 || i >= len(lines) {
		return ""
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[i]), "*/") {
		return ""
	}
	var collected []string
	for ; i >= 0; i-- {
		text := strings.TrimSpace(lines[i])
		collected = append(collected, text)
		if strings.HasPrefix(text, "/**") || strings.HasPrefix(text, "/*") {
			break
		}
	}
	if i < 0 || !strings.HasPrefix(strings.TrimSpace(lines[i]), "/*") {
		return ""
	}
	// collected is bottom-up; read it top-down.
	var parts []string
	for j := len(collected) - 1; j >= 0; j-- {
		text := collected[j]
		text = strings.TrimSuffix(strings.TrimPrefix(text, "/**"), "*/")
		text = strings.TrimSuffix(text, "*/")
		text = strings.TrimPrefix(text, "/*")
		text = strings.TrimSpace(text)
		text = strings.TrimPrefix(text, "*")
		text = strings.TrimSpace(text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

// APIMarkdown renders API rows as a padded markdown table.
func APIMarkdown(rows []MethodRow) string {
	cells := make([][]string, 0, len(rows))
	for _, r := range rows {
		cells = append(cells, []string{cell(r.Class), cell(r.Member), cell(r.Signature), cell(tableText(r.Doc))})
	}
	return markdownTable([]string{"Class", "Member", "Signature", "Doc"}, cells)
}

// tableText makes a doc safe for a markdown table cell: a pipe would end the
// cell and a newline would end the row.
func tableText(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
