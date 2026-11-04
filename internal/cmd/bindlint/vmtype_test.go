package main

import (
	"go/ast"
	"go/parser"
	"testing"
)

// parseType parses a type expression, the shape a parameter or a type switch
// case holds.
func parseType(t *testing.T, src string) ast.Expr {
	t.Helper()
	e, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return e
}

// TestVMType covers the wrappers a runtime value type hides behind: a pointer,
// a slice, a map value, a variadic slot. An Array another package declares is
// no runtime value type, and neither is model's every other type.
func TestVMType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src  string
		want string
	}{
		{"*model.Array", "model.Array"},
		{"model.Array", "model.Array"},
		{"*model.Object", "model.Object"},
		{"[]*model.Array", "model.Array"},
		{"map[string]*model.Object", "model.Object"},
		{"map[*model.Array]string", "model.Array"},
		{"...*model.Array", "model.Array"},
		{"any", ""},
		{"[]any", ""},
		{"map[string]any", ""},
		{"*model.Program", ""},
		{"*ast.Array", ""},
		{"string", ""},
	}
	for _, tc := range cases {
		src := tc.src
		if len(src) > 3 && src[:3] == "..." {
			// An ellipsis parses as a type only inside a parameter list.
			ftype := parseType(t, "func(a "+src+")").(*ast.FuncType)
			if got := vmType(ftype.Params.List[0].Type); got != tc.want {
				t.Errorf("vmType(%q) is %q, want %q", src, got, tc.want)
			}
			continue
		}
		if got := vmType(parseType(t, src)); got != tc.want {
			t.Errorf("vmType(%q) is %q, want %q", src, got, tc.want)
		}
	}
}

// TestNativeShape covers the cases that tell a type switch it already answers
// for the value a binding built.
func TestNativeShape(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"[]any":            true,
		"[]string":         true,
		"map[string]any":   true,
		"[]map[string]any": true,
		"*model.Array":     false,
		"string":           false,
		"BindingRecord":    false,
		"*BindingRecord":   false,
	}
	for src, want := range cases {
		if got := nativeShape(parseType(t, src)); got != want {
			t.Errorf("nativeShape(%q) is %v, want %v", src, got, want)
		}
	}
}

// TestCallNames covers the two names a call carries: what it calls, and the
// variable it calls it on.
func TestCallNames(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src      string
		called   string
		receiver string
	}{
		// A package qualifier reads as a receiver and costs no finding: a
		// receiver answers against model.Array's own method names.
		{"model.RangeValues(v, fn)", "RangeValues", "model"},
		{"options.Get(\"path\")", "Get", "options"},
		{"arrayTarget(name, array)", "arrayTarget", ""},
		{"a.b.Set(k, v)", "Set", ""},
		{"fn()", "fn", ""},
		{"func() {}()", "", ""},
	}
	for _, tc := range cases {
		call, ok := parseType(t, tc.src).(*ast.CallExpr)
		if !ok {
			t.Fatalf("%q is no call", tc.src)
		}
		if got := calledName(call); got != tc.called {
			t.Errorf("calledName(%q) is %q, want %q", tc.src, got, tc.called)
		}
		if got := receiverName(call); got != tc.receiver {
			t.Errorf("receiverName(%q) is %q, want %q", tc.src, got, tc.receiver)
		}
	}
}

// TestPHPName covers the spelling a finding reports a namespaced class under:
// the Go source escapes the separator and a script types one backslash.
func TestPHPName(t *testing.T) {
	t.Parallel()
	if got, want := phpName(`Runtime\\Router`), `Runtime\Router`; got != want {
		t.Errorf("phpName is %q, want %q", got, want)
	}
	if got, want := phpName("strtr"), "strtr"; got != want {
		t.Errorf("phpName is %q, want %q", got, want)
	}
}
