package runner

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestFlatHostBinary covers the operator table the bytecode engine calls back
// into. Both engines read one definition of PHP's semantics, so `==` cannot
// match on a rule the interpreter disagrees with; an operator the table has no
// entry for is an error and never a silent zero.
func TestFlatHostBinary(t *testing.T) {
	t.Parallel()
	host := flatHost{runtime: New(io.Discard, Options{})}
	for _, tc := range []struct {
		op   string
		a, b any
		want any
	}{
		{".", "n=", int64(7), "n=7"},
		{"+", int64(2), int64(3), int64(5)},
		{"-", int64(2), int64(3), int64(-1)},
		{"*", int64(2), int64(3), int64(6)},
		{"%", int64(7), int64(3), int64(1)},
		{"**", int64(2), int64(3), int64(8)},
		{"&", int64(6), int64(3), int64(2)},
		{"<<", int64(1), int64(3), int64(8)},
		{"==", "1", "01", true},
		{"!==", int64(1), 1.0, true},
		{"<", int64(1), int64(2), true},
		{"&&", int64(1), "", false},
		{"||", int64(0), "yes", true},
		{"instanceof", model.NewObject(&model.Class{Name: "Store"}), "Store", true},
	} {
		got, err := host.Binary(tc.op, tc.a, tc.b)
		if err != nil {
			t.Errorf("Binary(%q) failed: %v", tc.op, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Binary(%q, %#v, %#v) = %#v, want %#v", tc.op, tc.a, tc.b, got, tc.want)
		}
	}

	if _, err := host.Binary("??", int64(1), int64(1)); err == nil {
		t.Error("an unsupported binary operator reported no error")
	}
}

// TestFlatHostUnary covers the prefix operators the engine calls back for.
// Unary `+` resolves through the arithmetic helper, so a numeric string reads
// as the number it spells, and `-` keeps the sign of negative zero.
func TestFlatHostUnary(t *testing.T) {
	t.Parallel()
	host := flatHost{runtime: New(io.Discard, Options{})}
	for _, tc := range []struct {
		op    string
		value any
		want  any
	}{
		{"!", "", true},
		{"!", "yes", false},
		{"+", "5", int64(5)},
		{"-", int64(5), int64(-5)},
		{"~", int64(5), int64(-6)},
	} {
		got, err := host.Unary(tc.op, tc.value)
		if err != nil {
			t.Errorf("Unary(%q) failed: %v", tc.op, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Unary(%q, %#v) = %#v, want %#v", tc.op, tc.value, got, tc.want)
		}
	}

	if _, err := host.Unary("?", int64(1)); err == nil {
		t.Error("an unsupported unary operator reported no error")
	}
}

// TestFlatHostEntries covers what the engine's foreach walks. The walk takes an
// array and an object in the order they read back, and a Go slice or map a
// binding returned too, so `foreach` over a binding's result behaves the way it
// behaves in the interpreter.
func TestFlatHostEntries(t *testing.T) {
	t.Parallel()
	host := flatHost{runtime: New(io.Discard, Options{})}

	array := model.NewArray()
	array.Set("a", int64(1))
	array.Set("b", int64(2))
	if got := host.Entries(array); len(got) != 2 || got[0].Key != "a" || got[1].Value != int64(2) {
		t.Errorf("Entries() over an array = %#v", got)
	}

	obj := model.NewObject(&model.Class{Fields: []model.Field{{Name: "id"}, {Name: "name"}}})
	obj.SetProp("name", "ada")
	obj.SetProp("id", int64(7))
	entries := host.Entries(obj)
	if len(entries) != 2 || entries[0].Key != "id" || entries[1].Key != "name" {
		t.Errorf("Entries() over an object = %#v, want the declared order", entries)
	}

	if got := host.Entries([]string{"a", "b"}); len(got) != 2 || got[1].Key != int64(1) {
		t.Errorf("Entries() over a slice = %#v", got)
	}
	if got := host.Entries(map[string]int{"n": 1}); len(got) != 1 || got[0].Key != "n" {
		t.Errorf("Entries() over a map = %#v", got)
	}
	if got := host.Entries(nil); got != nil {
		t.Errorf("Entries(nil) = %#v, want nothing", got)
	}
}

// TestFlatHostSetEntry covers the by-reference foreach write-back. Only a
// model.Array is script-owned storage; a native Go collection a binding
// returned belongs to the host, so SetEntry drops the write and reports
// nothing. Runtime.execForeach takes the same view.
func TestFlatHostSetEntry(t *testing.T) {
	t.Parallel()
	host := flatHost{runtime: New(io.Discard, Options{})}

	array := model.NewArray()
	array.Append("a")
	if err := host.SetEntry(array, int64(0), "b"); err != nil {
		t.Fatalf("SetEntry failed: %v", err)
	}
	if value, _ := array.Get(int64(0)); value != "b" {
		t.Errorf("the entry holds %#v after SetEntry, want %q", value, "b")
	}

	host.SetEntry(array, "1", "second")
	if value, _ := array.Get(int64(1)); value != "second" {
		t.Errorf("a string key was not normalised: the array holds %#v at 1", value)
	}

	native := []string{"a"}
	if err := host.SetEntry(native, int64(0), "b"); err != nil {
		t.Errorf("SetEntry on a host-owned slice reported %v, want no error", err)
	}
	if native[0] != "a" {
		t.Errorf("the host-owned slice was written: %#v", native)
	}
}

// TestFlatHostThrow covers turning a thrown value into the error it travels
// as. An error travels as itself, an instance travels in the wrapper a catch
// clause reads its class off, and anything else renders as an uncaught
// exception.
func TestFlatHostThrow(t *testing.T) {
	t.Parallel()
	host := flatHost{runtime: New(io.Discard, Options{})}

	thrown := errors.New("no such table")
	if got := host.Throw(thrown); !errors.Is(got, thrown) {
		t.Errorf("Throw() = %v, want the error itself", got)
	}

	obj := model.NewObject(&model.Class{Name: "AppException"})
	wrapped := host.Throw(obj)
	class, ok := throwableClassOf(wrapped)
	if !ok || class != "AppException" {
		t.Errorf("Throw() on an instance answered class %q, want AppException", class)
	}
	if got := host.CatchValue(wrapped); got != any(obj) {
		t.Error("CatchValue() did not answer the thrown instance")
	}

	if got, want := host.Throw("text").Error(), "uncaught exception: text"; got != want {
		t.Errorf("Throw() on a scalar = %q, want %q", got, want)
	}
}

// TestFlatHostEcho covers bytecode output going through the runtime output
// stack and never to the base writer, so ob_start captures what the engine
// emits the way it captures interpreted output.
func TestFlatHostEcho(t *testing.T) {
	t.Parallel()
	base := &bytes.Buffer{}
	rt := New(base, Options{})
	host := flatHost{runtime: rt}

	if err := host.Echo(int64(7)); err != nil {
		t.Fatalf("Echo failed: %v", err)
	}
	if got, want := base.String(), "7"; got != want {
		t.Errorf("the base writer holds %q, want %q", got, want)
	}

	var captured bytes.Buffer
	rt.PushOutput(&captured)
	if err := host.Echo("captured"); err != nil {
		t.Fatalf("Echo failed: %v", err)
	}
	if got, want := captured.String(), "captured"; got != want {
		t.Errorf("the pushed writer holds %q, want %q", got, want)
	}
	if got, want := base.String(), "7"; got != want {
		t.Errorf("the base writer holds %q while a redirection is active, want %q", got, want)
	}
}

// TestFlatHostSetProperty covers writing an object property the way the
// interpreter writes one: a PHP object carries the value in its property map,
// and a Go binding assigns the struct field the name resolves to, so
// `$db->is_readonly = true` sets IsReadonly.
func TestFlatHostSetProperty(t *testing.T) {
	t.Parallel()
	host := flatHost{runtime: New(io.Discard, Options{})}

	obj := model.NewObject(&model.Class{Name: "Counter"})
	if err := host.SetProperty(obj, "total", int64(2), "="); err != nil {
		t.Fatalf("SetProperty failed: %v", err)
	}
	if value, _ := obj.Prop("total"); value != int64(2) {
		t.Errorf("the property holds %#v, want 2", value)
	}

	if err := host.SetProperty(obj, "total", int64(3), "+="); err != nil {
		t.Fatalf("a compound assignment failed: %v", err)
	}
	if value, _ := obj.Prop("total"); value != int64(5) {
		t.Errorf("the property holds %#v after `+=`, want 5", value)
	}

	if got := host.GetProperty(obj, "total"); got != int64(5) {
		t.Errorf("GetProperty() = %#v, want 5", got)
	}

	if err := host.UnsetProperty(obj, "total"); err != nil {
		t.Fatalf("UnsetProperty failed: %v", err)
	}
	if _, ok := obj.Prop("total"); ok {
		t.Error("the property is still set after UnsetProperty")
	}
	if err := host.UnsetProperty("text", "total"); err != nil {
		t.Errorf("UnsetProperty on a scalar reported %v, want no error", err)
	}
}

// TestFlatHostMemoryCheckInterval covers how often the engine polls the memory
// limit. A runtime with no limit configured answers zero, so the engine runs
// the poll out of its loop altogether.
func TestFlatHostMemoryCheckInterval(t *testing.T) {
	t.Parallel()
	unbounded := flatHost{runtime: New(io.Discard, Options{})}
	if got := unbounded.MemoryCheckInterval(); got != 0 {
		t.Errorf("MemoryCheckInterval() = %d with no limit, want 0", got)
	}

	bounded := flatHost{runtime: New(io.Discard, Options{MemoryLimit: Size(1 << 20)})}
	if got, want := bounded.MemoryCheckInterval(), memCheckInstructions; got != want {
		t.Errorf("MemoryCheckInterval() = %d with a limit, want %d", got, want)
	}
}

// TestFlatHostHelpers covers the helpers the engine shares with the
// interpreter: the array constructor, index access, truthiness, the casts.
// There is one definition of each, so neither engine can answer a different
// value for the same expression.
func TestFlatHostHelpers(t *testing.T) {
	t.Parallel()
	host := flatHost{runtime: New(io.Discard, Options{})}

	array, ok := host.Array([]model.ArrayItemValue{
		{Val: "first"},
		{Key: "name", Val: "ada"},
	}).(*model.Array)
	if !ok {
		t.Fatal("Array() answered something other than an array")
	}
	if value, _ := array.Get(int64(0)); value != "first" {
		t.Errorf("the unkeyed item landed at %#v, want key 0", value)
	}
	if got := host.Index(array, "name"); got != "ada" {
		t.Errorf("Index() = %#v, want the keyed value", got)
	}
	if got := host.Index(array, "absent"); got != nil {
		t.Errorf("Index() on a missing key = %#v, want nil", got)
	}

	if host.Truthy(model.NewArray()) {
		t.Error("Truthy() = true on an empty array")
	}
	if !host.Truthy(array) {
		t.Error("Truthy() = false on a filled array")
	}
	if got := host.Cast("int", "12abc"); got != int64(12) {
		t.Errorf(`Cast("int", "12abc") = %#v, want 12`, got)
	}
}

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
