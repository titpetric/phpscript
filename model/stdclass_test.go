package model

import (
	"testing"
)

// TestStdClass covers the one class every stdClass instance points at. Two
// instances share it, so a class-name comparison reads one class and no two
// classes spelled alike.
//
// It lives as a package-level value and no entry in the runtime's class table.
// The runtime clears that table between runs, so every reset would have to
// seed the entry again; and RegisterClass merges into an existing entry, so a
// script declaring its own `class stdClass` would write its methods onto the
// one every other instance points at. Declared here, the script's declaration
// shadows it in the table and this value is never reached.
func TestStdClass(t *testing.T) {
	t.Parallel()
	class := StdClass()
	if class != StdClass() {
		t.Fatal("two calls answered two classes")
	}
	if got := class.Name; got != "stdClass" {
		t.Errorf("Name = %q, want \"stdClass\"", got)
	}
	if got := len(class.Fields); got != 0 {
		t.Errorf("stdClass declares %d fields, want 0", got)
	}
	if got := len(class.Methods); got != 0 {
		t.Errorf("stdClass declares %d methods, want 0", got)
	}
	if got := len(class.Consts); got != 0 {
		t.Errorf("stdClass declares %d constants, want 0", got)
	}
	if got := len(class.Statics); got != 0 {
		t.Errorf("stdClass declares %d static properties, want 0", got)
	}
	if got := len(class.Implements); got != 0 {
		t.Errorf("stdClass declares %d interfaces, want 0", got)
	}
}

// TestNewStdClass covers the instance `new stdClass` and the `(object)` cast
// both produce: empty, pointing at the shared class. Instances share the class
// and no property bag, so a property written on one instance is invisible on
// another.
func TestNewStdClass(t *testing.T) {
	t.Parallel()
	first, second := NewStdClass(), NewStdClass()

	if first == second {
		t.Fatal("two calls answered one instance")
	}
	if first.Class != StdClass() || second.Class != StdClass() {
		t.Error("an instance points at a class other than the shared one")
	}
	if got := first.Len(); got != 0 {
		t.Errorf("a fresh instance holds %d properties, want 0", got)
	}

	first.SetProp("id", int64(1))
	if got, ok := first.Prop("id"); !ok || got != int64(1) {
		t.Errorf(`Prop("id") = (%v, %v), want (1, true)`, got, ok)
	}
	if _, ok := second.Prop("id"); ok {
		t.Error("a property written on one instance is readable on another")
	}
	if got := len(StdClass().Fields); got != 0 {
		t.Errorf("the shared class declares %d fields after a dynamic property, want 0", got)
	}
}
