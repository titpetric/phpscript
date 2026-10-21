package runner

import (
	"io"
	"testing"
)

// TestFlatHostFrame covers the handle the engine binds once per run, which
// replaced the map copied in and out around every call. A host with no frame
// bound materialises an empty scope, so a callee reading caller locals before
// the bind sees nothing and never a nil map.
func TestFlatHostFrame(t *testing.T) {
	t.Parallel()
	host := &flatHost{runtime: New(io.Discard, Options{})}
	if host.TakeFrame() != nil {
		t.Error("a fresh host carries a frame")
	}

	scope := host.boundScope()
	if scope == nil {
		t.Fatal("boundScope() answered nothing with no frame bound")
	}
	if got := len(scope.DefinedVars()); got != 0 {
		t.Errorf("the materialised scope holds %d variables, want 0", got)
	}

	// With no frame bound there is nowhere to write back to, so pullScope
	// leaves the materialised scope as it found it.
	scope.Set("rows", int64(3))
	host.pullScope(scope)
	if value, ok := scope.Get("rows"); !ok || value != int64(3) {
		t.Errorf("pullScope with no frame bound changed the scope: %#v", value)
	}
}

// TestFlatHostCall covers the engine's function-call path: a function-table
// hit dispatches through the cached entry, a missing name resolves through
// the fallback, and a name neither table holds reports the call as undefined.
func TestFlatHostCall(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})
	rt.RegisterFunc("double", func(n int64) int64 { return n * 2 })
	host := &flatHost{runtime: rt}

	got, err := host.Call("double", "", []any{int64(3)})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if got != int64(6) {
		t.Errorf("Call() = %#v, want 6", got)
	}

	got, err = host.Call("missing", "double", []any{int64(4)})
	if err != nil {
		t.Fatalf("Call through the fallback failed: %v", err)
	}
	if got != int64(8) {
		t.Errorf("Call() through the fallback = %#v, want 8", got)
	}

	if _, err := host.Call("missing", "", nil); err == nil {
		t.Error("an undefined function reported no error")
	}
}
