package model

import (
	"testing"
)

// TestClass_Field covers reading a declaration back off a class. A caller
// reaches for it when the name alone is not enough: the formatter prints the
// visibility and the type hint an author wrote, and the runtime refuses a write
// to a private property from outside.
//
// A name the class does not declare answers the zero Field and false, which is
// what separates an undeclared property from one declared with no visibility.
func TestClass_Field(t *testing.T) {
	class := &Class{
		Name: "Invoice",
		Fields: []Field{
			{Name: "total", Visibility: "public", Type: "int"},
			{Name: "secret", Visibility: "private"},
		},
	}

	for _, tc := range []struct {
		name       string
		found      bool
		visibility string
		hint       string
	}{
		{name: "total", found: true, visibility: "public", hint: "int"},
		{name: "secret", found: true, visibility: "private"},
		{name: "missing"},
	} {
		field, ok := class.Field(tc.name)
		if ok != tc.found {
			t.Errorf("Field(%q) found = %v, want %v", tc.name, ok, tc.found)
			continue
		}
		if !ok {
			if field.Name != "" {
				t.Errorf("Field(%q) answered %#v with false, want the zero Field", tc.name, field)
			}
			continue
		}
		if field.Name != tc.name {
			t.Errorf("Field(%q).Name = %q", tc.name, field.Name)
		}
		if field.Visibility != tc.visibility {
			t.Errorf("Field(%q).Visibility = %q, want %q", tc.name, field.Visibility, tc.visibility)
		}
		if field.Type != tc.hint {
			t.Errorf("Field(%q).Type = %q, want %q", tc.name, field.Type, tc.hint)
		}
	}
}

// TestClass_declares covers the check a property write passes through. It
// answers the declaration and not the value, so a class whose property is unset
// still declares it.
func TestClass_declares(t *testing.T) {
	class := &Class{Fields: []Field{{Name: "total"}}}
	if !class.declares("total") {
		t.Error(`declares("total") = false on a class declaring it`)
	}
	if class.declares("Total") {
		t.Error(`declares("Total") = true, and PHP property names are case sensitive`)
	}
	if (&Class{}).declares("total") {
		t.Error("a class declaring no field declares one")
	}
}
