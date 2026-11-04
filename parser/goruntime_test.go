package parser

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// goCall returns the call the last statement of src compiled to. A namespaced
// file declares only functions, so the statement is read out of the last
// declaration where there is one.
func goCall(t *testing.T, src string) *model.Call {
	t.Helper()
	prog := mustParse(t, src)
	last := prog.Stmts[len(prog.Stmts)-1]
	if fn, ok := last.(*model.FuncDecl); ok {
		last = fn.Body[len(fn.Body)-1]
	}
	stmt, ok := last.(*model.ExprStmt)
	if !ok {
		t.Fatalf("%s: last statement is a %T, want an expression", src, last)
	}
	call, ok := stmt.X.(*model.Call)
	if !ok {
		t.Fatalf("%s: the expression is a %T, want a call", src, stmt.X)
	}
	return call
}

// TestGoRuntimeBinding covers what the import binds and what it leaves alone: a
// go builtin written bare in an importing file, and nothing else. The call
// keeps the source's spelling as its fallback, which is the name php would have
// resolved and the name the formatter prints.
func TestGoRuntimeBinding(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ src, name, fallback string }{
		{"<?php\nuse Runtime\\Go;\nmake(1);", `Runtime\Go\make`, "make"},
		{"<?php\nuse Runtime\\Go;\nint64(1);", `Runtime\Go\int64`, "int64"},
		// A bare name no import binds stays php's, with or without a namespace.
		{"<?php\nmake(1);", "make", ""},
		{"<?php\nuse Runtime\\Go;\nstrlen(1);", "strlen", ""},
		{"<?php\nuse Runtime\\Router;\nmake(1);", "make", ""},
		{"<?php\nnamespace App;\nfunction f() { make(1); }", `App\make`, "make"},
		// The import wins over the enclosing namespace, as php reads an
		// imported name, and a leading separator and the case of the namespace
		// are php's to ignore.
		{"<?php\nnamespace App;\nuse Runtime\\Go;\nfunction f() { make(1); }", `Runtime\Go\make`, "make"},
		{"<?php\nuse \\runtime\\GO;\nmake(1);", `Runtime\Go\make`, "make"},
		{"<?php\nuse Runtime\\Go as G;\nmake(1);", `Runtime\Go\make`, "make"},
		// go spells its builtins one way. A file that writes another asked php
		// for the name, and a qualified call decided for itself.
		{"<?php\nuse Runtime\\Go;\nMAKE(1);", "MAKE", ""},
		{"<?php\nuse Runtime\\Go;\n\\make(1);", "make", ""},
		{"<?php\nuse Runtime\\Go;\nGo\\make(1);", `Runtime\Go\make`, ""},
	} {
		call := goCall(t, test.src)
		if call.Name != test.name || call.Fallback != test.fallback {
			t.Errorf("%q = %s / %s, want %s / %s", test.src, call.Name, call.Fallback, test.name, test.fallback)
		}
	}
}

// TestGoRuntimeImportSpellings covers the three ways a file can name the
// namespace in a `use`, which the parser reads for the binding and the runtime
// reads for the install.
func TestGoRuntimeImportSpellings(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		src  string
		want bool
	}{
		{"<?php\nuse Runtime\\Go;\n", true},
		{"<?php\nuse Runtime\\Go\\make;\n", false},
		{"<?php\nuse function Runtime\\Go\\make;\n", false},
		{"<?php\nuse Runtime\\Gone;\n", false},
	} {
		p := &parser{}
		p.useGo(importPath(t, test.src))
		if p.goRuntime != test.want {
			t.Errorf("%q binds go's builtins: %v, want %v", test.src, p.goRuntime, test.want)
		}
	}
}

// importPath returns the path the first import of src names.
func importPath(t *testing.T, src string) string {
	t.Helper()
	prog := mustParse(t, src)
	use, ok := prog.Stmts[0].(*model.Use)
	if !ok || len(use.Imports) == 0 {
		t.Fatalf("%s: the first statement imports nothing", src)
	}
	return use.Imports[0].Path
}
