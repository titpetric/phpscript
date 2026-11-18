package main

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// goPackage holds a standard library package's symbol table, read from the
// source tree so a link names a symbol pkg.go.dev renders.
type goPackage struct {
	names   map[string]bool
	methods map[string]map[string]bool
}

// has reports a package's top-level function, constant, variable or type.
func (p *goPackage) has(name string) bool { return p.names[name] }

// hasMethod reports a method of the named type.
func (p *goPackage) hasMethod(kind, method string) bool {
	return p.methods[kind][method]
}

// loadPackage reads the package's top-level names and methods out of GOROOT.
// It refuses a path the toolchain does not hold, a binding naming a linked
// package. This run sees no source tree for it.
func loadPackage(path string) (*goPackage, error) {
	dir := filepath.Join(build.Default.GOROOT, "src", filepath.FromSlash(path))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	pkg := &goPackage{names: map[string]bool{}, methods: map[string]map[string]bool{}}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		pkg.collect(file)
	}
	return pkg, nil
}

// collect files a source file's declarations into the package table.
func (p *goPackage) collect(file *ast.File) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				p.names[d.Name.Name] = true
				continue
			}
			kind := receiverName(d.Recv)
			if kind == "" {
				continue
			}
			if p.methods[kind] == nil {
				p.methods[kind] = map[string]bool{}
			}
			p.methods[kind][d.Name.Name] = true
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					p.names[s.Name.Name] = true
				case *ast.ValueSpec:
					for _, name := range s.Names {
						p.names[name.Name] = true
					}
				}
			}
		}
	}
}

// receiverName answers the receiver's declared type. It answers empty for a
// receiver a package cannot name: an anonymous or a generic instance.
func receiverName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	for {
		switch t := expr.(type) {
		case *ast.StarExpr:
			expr = t.X
		case *ast.IndexExpr:
			expr = t.X
		case *ast.IndexListExpr:
			expr = t.X
		case *ast.ParenExpr:
			expr = t.X
		case *ast.Ident:
			return t.Name
		default:
			return ""
		}
	}
}
