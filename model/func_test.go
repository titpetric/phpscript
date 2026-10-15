package model

import (
	"testing"
)

// TestFunc covers the two shapes a callable value takes. A PHP declaration
// carries Decl and a host function carries Go, and the runtime reads whichever
// is set to decide how to invoke it.
func TestFunc(t *testing.T) {
	t.Parallel()
	declared := Func{Decl: &FuncDecl{Name: "greet"}}
	if declared.Decl == nil || declared.Decl.Name != "greet" {
		t.Errorf("a declared Func lost its declaration: %#v", declared)
	}
	if declared.Go != nil {
		t.Error("a declared Func carries a Go value")
	}

	host := Func{Go: func(name string) string { return name }}
	if host.Go == nil {
		t.Error("a host Func lost its Go value")
	}
	if host.Decl != nil {
		t.Error("a host Func carries a declaration")
	}

	if zero := (Func{}); zero.Decl != nil || zero.Go != nil {
		t.Errorf("the zero Func is not empty: %#v", zero)
	}
}
