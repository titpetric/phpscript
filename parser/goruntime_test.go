package parser

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// goCall returns the call the last statement of src compiled to. A namespaced
// file declares functions alone, and the statement then comes out of the last
// declaration.
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

// TestGoRuntimeBinding covers what the import binds and what it leaves alone:
// a bare go builtin in an importing file, and no other name. The call keeps
// the source's spelling as its fallback, the name php would have resolved and
// the name the formatter prints.
func TestGoRuntimeBinding(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ src, name, fallback string }{
		{"<?php\nuse Runtime\\Go;\nmake(1);", `Runtime\Go\make`, "make"},
		{"<?php\nuse Runtime\\Go;\nint64($n);", `Runtime\Go\int64`, "int64"},
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

// TestGoRuntimeFold covers the conversion of a literal, which the parser
// applies and no engine sees: a literal stands where the call stood, under
// every width go has, holding the type the width names. A conversion php has
// to read the value for stays a call at run time; so does a call over
// anything but one literal.
func TestGoRuntimeFold(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		src  string
		want any
	}{
		{"int64(0)", int64(0)},
		{"int8(300)", int8(44)},
		{"uint8(-1)", uint8(255)},
		{"float64(-1.5)", float64(-1.5)},
		{"int64((7))", int64(7)},
		{"int32(2147483648)", int32(-2147483648)},
		{"float64(3)", float64(3)},
		{"float32(0.5)", float32(0.5)},
		{"string('x')", "x"},
		{"any('x')", "x"},
		{"rune(65)", int32(65)},
	} {
		src := "<?php\nuse Runtime\\Go;\n$x = " + test.src + ";"
		value := mustParse(t, src).Stmts[1].(*model.Assign).Value
		lit, ok := value.(*model.Lit)
		if !ok {
			t.Errorf("%s compiled to a %T, want a literal", test.src, value)
			continue
		}
		if lit.Value != test.want {
			t.Errorf("%s = %#v, want %#v", test.src, lit.Value, test.want)
		}
	}

	// A string literal that folds to itself keeps the node, and with it the
	// quoting the source used.
	lit := mustParse(t, "<?php\nuse Runtime\\Go;\n$x = string('x');").Stmts[1].(*model.Assign).Value.(*model.Lit)
	if lit.Raw != "'x'" {
		t.Errorf("the folded literal reads %q, want the source spelling", lit.Raw)
	}

	for _, src := range []string{
		`int64("12")`, `int64(1.5)`, `int64($n)`, `int64()`, `int64(1, 2)`, `int64(...)`,
		`len("abc")`, `make("[]int", 1)`,
	} {
		call := goCall(t, "<?php\nuse Runtime\\Go;\n"+src+";")
		if call.Fallback == "" {
			t.Errorf("%s compiled to a call to %s, want the go binding", src, call.Name)
		}
	}
}

// TestGoRuntimeMakeType covers make's first argument, the one place a type
// stands where php puts an expression. The binding reads the spelling with a
// class name qualified, and the formatter prints the spelling the source wrote.
func TestGoRuntimeMakeType(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ src, value, raw string }{
		{"<?php\nuse Runtime\\Go;\nmake([]string, 0, 10);", "[]string", "[]string"},
		{"<?php\nuse Runtime\\Go;\nmake([][]int64, 1);", "[][]int64", "[][]int64"},
		{"<?php\nuse Runtime\\Go;\nmake([]User, 1);", "[]User", "[]User"},
		{"<?php\nnamespace App;\nuse Runtime\\Go;\nfunction f() { make([]User, 1); }", `[]App\User`, "[]User"},
		{"<?php\nuse Runtime\\Go;\nmake([]\\Other\\User, 1);", `[]Other\User`, `[]Other\User`},
		// A class the file imported qualifies as every other name does. The
		// source keeps the short spelling it wrote.
		{"<?php\nuse Runtime\\Go;\nuse Other\\User;\nmake([]User, 1);", `[]Other\User`, "[]User"},
		// The type is a string a script may write by hand, which is the same
		// call and takes the same path to the binding.
		{`<?php` + "\nuse Runtime\\Go;\nmake(\"[]string\", 1);", "[]string", `"[]string"`},
	} {
		call := goCall(t, test.src)
		if call.Name != `Runtime\Go\make` {
			t.Errorf("%q calls %s, want the make binding", test.src, call.Name)
			continue
		}
		lit, ok := call.Args[0].(*model.Lit)
		if !ok {
			t.Errorf("%q: the type argument is a %T, want a literal", test.src, call.Args[0])
			continue
		}
		if lit.Value != test.value || lit.Raw != test.raw {
			t.Errorf("%q = %q / %q, want %q / %q", test.src, lit.Value, lit.Raw, test.value, test.raw)
		}
	}

	// A length and a capacity are expressions. The call takes them as any
	// other call takes its arguments.
	call := goCall(t, "<?php\nuse Runtime\\Go;\nmake([]int, count($a), 2 * $n);")
	if len(call.Args) != 3 {
		t.Errorf("make([]int, count($a), 2 * $n) took %d arguments, want 3", len(call.Args))
	}

	for _, src := range []string{"make([], 1);", "make([]$type, 1);", "make([]int 1);"} {
		if _, err := Parse("<?php\nuse Runtime\\Go;\n" + src); err == nil {
			t.Errorf("%s parsed, want an error", src)
		}
	}
}

// TestGoRuntimeSliceHint covers the `[]T` type hint, which php has no spelling
// for and phpscript reads in a file that imported the go runtime: on a
// parameter, on a return type and on a property. No engine downstream reads a
// type; the parser keeps the spelling for the formatter to print back.
func TestGoRuntimeSliceHint(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ src, params, returns string }{
		{"function f([]int64 $a): []float64 {}", "[]int64", "[]float64"},
		{"function f([][]string $a): []Row {}", "[][]string", "[]Row"},
		{"function f(?[]int $a) {}", "?[]int", ""},
		{"function f([]\\Other\\Row $a) {}", `[]\Other\Row`, ""},
		// A bracket in a type position opens a type and no array literal, and
		// a hint php does spell still reads as it did.
		{"function f(array $a): string {}", "array", "string"},
	} {
		fn := mustParse(t, "<?php\nuse Runtime\\Go;\n"+test.src).Stmts[1].(*model.FuncDecl)
		if fn.Params[0].Type != test.params || fn.ReturnType != test.returns {
			t.Errorf("%s = %q / %q, want %q / %q", test.src, fn.Params[0].Type, fn.ReturnType, test.params, test.returns)
		}
	}

	// A property declares one too, and the class table is where a type hint
	// ends: the runtime reads none of them.
	class := mustParse(t, "<?php\nuse Runtime\\Go;\nclass Totals { public []int64 $counts; }").Stmts[1].(*model.ClassDecl)
	if class.Fields[0].Type != "[]int64" {
		t.Errorf("the property reads %q, want []int64", class.Fields[0].Type)
	}

	// The syntax comes with the import, as the bare names do: a file that
	// imported none of it reads php's grammar, which has no slice type.
	if _, err := Parse("<?php\nfunction f([]int $a) {}"); err == nil {
		t.Error("a slice hint parsed in a file that imported nothing")
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
