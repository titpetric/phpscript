package runner

import (
	"context"
	"io"
	"reflect"
	"testing"
)

// TestScope covers the flat variable table of one frame. PHP has no block
// scoping, so a variable introduced inside an if or a foreach body lives in the
// enclosing function scope, and one frame is one map.
func TestScope(t *testing.T) {
	scope := NewScope()
	if _, ok := scope.Get("rows"); ok {
		t.Error(`Get("rows") = true on a fresh scope`)
	}

	scope.Set("rows", int64(3))
	value, ok := scope.Get("rows")
	if !ok {
		t.Fatal(`Get("rows") = false after Set`)
	}
	if value != int64(3) {
		t.Errorf(`Get("rows") = %#v, want 3`, value)
	}

	scope.Set("rows", int64(4))
	if value, _ := scope.Get("rows"); value != int64(4) {
		t.Errorf(`Get("rows") = %#v after a second Set, want 4`, value)
	}
}

// TestScope_Unset covers PHP's unset. Removing a name nothing set is no error.
// Unsetting a static-bound name breaks the local link and leaves the stored
// value for the next call, as PHP leaves it.
func TestScope_Unset(t *testing.T) {
	scope := NewScope()
	scope.Set("rows", int64(3))
	scope.Unset("rows")
	if _, ok := scope.Get("rows"); ok {
		t.Error("the name is still set after Unset")
	}
	scope.Unset("never-set")

	bag := map[string]any{"count": int64(7)}
	scope.bindStatic("count", bag)
	if value, _ := scope.Get("count"); value != int64(7) {
		t.Errorf("a bound static reads %#v, want 7", value)
	}

	scope.Unset("count")
	if _, ok := scope.Get("count"); ok {
		t.Error("the static is still readable through the frame after Unset")
	}
	if bag["count"] != int64(7) {
		t.Errorf("the persistent bag holds %#v after Unset, want 7", bag["count"])
	}
}

// TestScope_DefinedVars covers what get_defined_vars() reports. An interpreter
// bookkeeping slot takes a double-underscore prefix and is left out, and so is
// `this`: the call binds it and no body declares it, so php leaves it out. A
// static-bound name reports the value in its persistent bag.
func TestScope_DefinedVars(t *testing.T) {
	scope := NewScope()
	scope.Set("rows", int64(3))
	scope.Set("this", "receiver")
	scope.Set("__FILE__", "/app/index.php")
	scope.bindStatic("count", map[string]any{"count": int64(7)})

	want := map[string]any{"rows": int64(3), "count": int64(7)}
	if got := scope.DefinedVars(); !reflect.DeepEqual(got, want) {
		t.Errorf("DefinedVars() = %#v, want %#v", got, want)
	}
}

// TestScope_Defer covers the callbacks a frame runs on return. PHP has no
// defer; this runtime does, and the order is last in, first out, so a release
// registered after an acquire runs before it.
func TestScope_Defer(t *testing.T) {
	scope := NewScope()
	if len(scope.deferred) != 0 {
		t.Fatalf("a fresh scope carries %d deferred callbacks", len(scope.deferred))
	}

	first := func() {}
	second := func() {}
	scope.Defer(first)
	scope.Defer(second)
	if len(scope.deferred) != 2 {
		t.Fatalf("the scope carries %d deferred callbacks, want 2", len(scope.deferred))
	}
	if reflect.ValueOf(scope.deferred[1]).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Error("the last registered callback is not last in the list")
	}
}

// TestScopeFromContext covers the frame a registered Go free function reads off
// its injected context. compact() and get_defined_vars() reach caller-local
// variables from a binding through it.
func TestScopeFromContext(t *testing.T) {
	if _, ok := ScopeFromContext(context.Background()); ok {
		t.Error("ScopeFromContext() = true on a context carrying no frame")
	}

	rt := New(io.Discard, Options{})
	scope := NewScope()
	scope.Set("rows", int64(3))
	ctx := rt.contextWithScope(context.Background(), scope)

	got, ok := ScopeFromContext(ctx)
	if !ok {
		t.Fatal("ScopeFromContext() = false on a context built with a frame")
	}
	if got != scope {
		t.Error("ScopeFromContext() answered another frame")
	}
}

// TestEnvFromContext covers the request-scoped environment a registered Go
// callable reads off its injected context. A virtual host declares its own
// variables, so getenv() inside a site reads that map and never the operator's
// process environment.
func TestEnvFromContext(t *testing.T) {
	if _, ok := EnvFromContext(context.Background()); ok {
		t.Error("EnvFromContext() = true on a context carrying no environment")
	}

	want := map[string]string{"APP_ENV": "testing"}
	ctx := contextWithEnv(context.Background(), want)
	got, ok := EnvFromContext(ctx)
	if !ok {
		t.Fatal("EnvFromContext() = false on a context built with an environment")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EnvFromContext() = %#v, want %#v", got, want)
	}
}
