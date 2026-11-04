package apidoc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestFieldComments covers which comment documents a property: the leading one
// and no other. A note trailing on the declaration line annotates the value
// and documents no member, and net/http's `ProtoMajor int // 1` would publish
// a docblock reading 1.
func TestFieldComments(t *testing.T) {
	t.Parallel()
	source := `package fake

type Holder struct {
	// ID is the connection the client opened under.
	ID string

	// IsReadonly restricts the client to statements that only read.
	//
	// It refuses insert(), replace() and update() outright.
	IsReadonly bool

	ProtoMajor int // 1

	Bare string
}
`
	comments := fieldComments(parseStructType(t, source))
	if got, want := len(comments["ID"]), 1; got != want {
		t.Errorf("ID documents %d lines, want %d: %q", got, want, comments["ID"])
	}
	if got, want := len(comments["IsReadonly"]), 3; got != want {
		t.Errorf("IsReadonly documents %d lines, want %d: %q", got, want, comments["IsReadonly"])
	}
	if got, ok := comments["ProtoMajor"]; ok {
		t.Errorf("a trailing note is published as a docblock: %q", got)
	}
	if got, ok := comments["Bare"]; ok {
		t.Errorf("an undocumented field reports %q, want nothing", got)
	}
}

// TestFieldCommentsNoStruct covers the declarations that carry no field: a
// named scalar, and a struct whose fields document nothing. Both answer nil, and
// a listing over them prints no docblock.
func TestFieldCommentsNoStruct(t *testing.T) {
	t.Parallel()
	if got := fieldComments(parseTypeExpr(t, "package fake\n\ntype Duration int64\n")); got != nil {
		t.Errorf("a named scalar reports %v, want nothing", got)
	}
	if got := fieldComments(parseStructType(t, "package fake\n\ntype Holder struct {\n\tID string\n}\n")); got != nil {
		t.Errorf("an undocumented struct reports %v, want nothing", got)
	}
}

// TestImportPath covers the key the doc index holds an entry under. A reflected
// type reports an import path and the scan walks directories, and the two have
// to agree or every lookup misses.
func TestImportPath(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                "github.com/titpetric/phpscript",
		".":               "github.com/titpetric/phpscript",
		"stdlib":          "github.com/titpetric/phpscript/stdlib",
		"stdlib/http":     "github.com/titpetric/phpscript/stdlib/http",
		"internal/apidoc": "github.com/titpetric/phpscript/internal/apidoc",
	}
	for dir, want := range cases {
		if got := importPath(dir); got != want {
			t.Errorf("importPath(%q) = %q, want %q", dir, got, want)
		}
	}
}

// parseStructType returns the struct type declaration of the first type in
// source, the expression fieldComments reads.
func parseStructType(t *testing.T, source string) ast.Expr {
	t.Helper()
	expr := parseTypeExpr(t, source)
	if _, ok := expr.(*ast.StructType); !ok {
		t.Fatalf("the first type declaration is %T, want a struct", expr)
	}
	return expr
}

func parseTypeExpr(t *testing.T, source string) ast.Expr {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "fake.go", source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				return ts.Type
			}
		}
	}
	t.Fatal("the source declares no type")
	return nil
}
