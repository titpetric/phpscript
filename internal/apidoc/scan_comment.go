package apidoc

import (
	"go/ast"
	"strings"
)

// stmtComment returns the comment written against the statement enclosing
// the current inspect position: the leading group when there is one, else a
// trailing comment on the same line.
func stmtComment(cmap ast.CommentMap, stack []ast.Node) []string {
	for i := len(stack) - 1; i >= 0; i-- {
		stmt, ok := stack[i].(ast.Stmt)
		if !ok {
			continue
		}
		groups := cmap[stmt]
		if len(groups) == 0 {
			return nil
		}
		for _, g := range groups {
			if g.End() < stmt.Pos() {
				return cleanComment(g)
			}
		}
		return cleanComment(groups[0])
	}
	return nil
}

// declComment returns a declaration's doc comment with the godoc-style
// leading symbol name replaced by the registered PHP name.
func declComment(decl *ast.FuncDecl, phpName string) []string {
	if decl.Doc == nil {
		return nil
	}
	lines := cleanComment(decl.Doc)
	if len(lines) > 0 {
		first, rest, ok := strings.Cut(lines[0], " ")
		if ok && first == decl.Name.Name {
			lines[0] = phpName + " " + rest
		}
	}
	return lines
}

// cleanComment strips comment markers and directives, returning plain lines.
func cleanComment(group *ast.CommentGroup) []string {
	var lines []string
	for _, c := range group.List {
		text := c.Text
		switch {
		case strings.HasPrefix(text, "//"):
			line := strings.TrimPrefix(strings.TrimPrefix(text, "//"), " ")
			if strings.HasPrefix(line, "go:") || strings.HasPrefix(line, "nolint") {
				continue
			}
			lines = append(lines, line)
		case strings.HasPrefix(text, "/*"):
			text = strings.TrimSuffix(strings.TrimPrefix(text, "/*"), "*/")
			for _, line := range strings.Split(text, "\n") {
				line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
				lines = append(lines, line)
			}
		}
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
