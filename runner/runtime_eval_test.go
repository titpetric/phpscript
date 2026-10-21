package runner

import (
	"io"
	"slices"
	"testing"
)

// TestScopeBuiltins covers the names installed per-environment and no entry in
// the function table. function_exists() and the linter both read that table, so
// a call to one of these would read as a call to something undefined if the
// list stayed unpublished.
func TestScopeBuiltins(t *testing.T) {
	t.Parallel()
	builtins := ScopeBuiltins()
	if len(builtins) == 0 {
		t.Fatal("ScopeBuiltins answered nothing")
	}
	if !slices.Contains(builtins, "func_get_args") {
		t.Errorf("ScopeBuiltins = %v, want func_get_args among them", builtins)
	}

	// Each name is resolvable in a scope and absent from the function table,
	// which is the split the list exists to bridge.
	rt := New(io.Discard, Options{})
	for _, name := range builtins {
		if _, ok := scopeBuiltin(name, NewScope()); !ok {
			t.Errorf("%s is listed and does not resolve in a scope", name)
		}
		if _, ok := rt.LookupFunc(name); ok {
			t.Errorf("%s is in the function table, so it needs no listing", name)
		}
	}
}
