package apidoc

import (
	"go/ast"
	"strings"
)

// fieldComments returns each named field's leading doc, keyed by field name.
func fieldComments(t ast.Expr) map[string][]string {
	st, ok := t.(*ast.StructType)
	if !ok || st.Fields == nil {
		return nil
	}
	out := map[string][]string{}
	for _, field := range st.Fields.List {
		if field.Doc == nil {
			continue
		}
		lines := cleanComment(field.Doc)
		if len(lines) == 0 {
			continue
		}
		for _, ident := range field.Names {
			out[ident.Name] = lines
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// importPath resolves a directory relative to the module root to its import path.
func importPath(dir string) string {
	if dir == "" || dir == "." {
		return strings.TrimSuffix(modulePrefix, "/")
	}
	return modulePrefix + dir
}
