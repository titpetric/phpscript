package apidoc

import (
	"go/ast"
	gobuild "go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// gorootDocs reads the source text of one standard library type's members out
// of the Go distribution: the doc comment of every exported field and method,
// with the parameter names each method declares.
//
// A class binding net/http.Request holds 22 properties and 20 methods whose
// godoc is in the distribution and not in this module, and the scan of the
// module tree reaches none of it. Reading the distribution is how
// `phpscript list --classes` answers about such a class what `go doc` answers.
//
// It answers nil when the distribution is not on disk, which is a toolchain
// the generator is not running under, and the listing then prints the
// structure reflection sees and no prose.
func gorootDocs(pkg, typeName string) *TypeDocs {
	dir := filepath.Join(gobuild.Default.GOROOT, "src", filepath.FromSlash(pkg))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	out := &TypeDocs{Fields: map[string][]string{}, Methods: map[string]*SymbolDocs{}}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			continue
		}
		out.collectGoroot(file, typeName)
	}
	if len(out.Fields) == 0 && len(out.Methods) == 0 {
		return nil
	}
	return out
}

// collectGoroot records what one parsed file says about typeName.
func (d *TypeDocs) collectGoroot(file *ast.File, typeName string) {
	for _, decl := range file.Decls {
		switch node := decl.(type) {
		case *ast.FuncDecl:
			if node.Recv == nil || len(node.Recv.List) == 0 {
				continue
			}
			if recvTypeName(node.Recv.List[0].Type) != typeName || !node.Name.IsExported() {
				continue
			}
			entry := &SymbolDocs{Comment: declComment(node, camelToSnake(node.Name.Name))}
			params, _ := signature(node.Type)
			entry.Params = paramNames(params)
			d.Methods[node.Name.Name] = entry
		case *ast.GenDecl:
			d.collectGorootType(node, typeName)
		}
	}
}

// collectGorootType records the field comments of the type declaration, which
// holds its own doc or inherits the declaration block's.
func (d *TypeDocs) collectGorootType(decl *ast.GenDecl, typeName string) {
	for _, spec := range decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok || ts.Name.Name != typeName {
			continue
		}
		for name, lines := range fieldComments(ts.Type) {
			d.Fields[name] = lines
		}
	}
}
