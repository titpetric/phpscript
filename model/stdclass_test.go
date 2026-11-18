package model

import (
	"testing"
)

// TestStdClass covers the single class behind every stdClass instance. Two
// instances share the class. A class-name comparison reads one class, since
// both spellings agree.
//
// It lives as a package-level value and holds no entry in the runtime's class
// table. The runtime clears that table between runs. Every reset would have to
// seed the entry again. RegisterClass merges into an existing entry. A script
// declaring `class stdClass` would then write its methods onto the shared
// class. Declared here, the script's declaration shadows it in the
// table. This value is never reached.
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
// only. A property written on one instance is invisible on another.
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
