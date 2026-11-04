package apidoc

import (
	"go/ast"
	"strings"
)

// fieldComments reads the doc comment of every named field of a struct type
// declaration, keyed by the Go field name. A type that is no struct, and a
// struct whose fields carry no comment, both answer nil.
//
// The leading comment is the only one it reads. A note trailing on the
// declaration line annotates the value and documents no member, and net/http's
// `ProtoMajor int // 1` would publish a docblock reading 1.
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

// importPath turns a package directory relative to the module root into the
// import path a reflected type reports.
func importPath(dir string) string {
	if dir == "" || dir == "." {
		return strings.TrimSuffix(modulePrefix, "/")
	}
	return modulePrefix + dir
}
