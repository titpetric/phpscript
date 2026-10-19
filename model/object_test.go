package model

import (
	"reflect"
	"testing"
)

// TestObject covers constructing an instance and reading a property back.
// NewObject takes the class and leaves Props unallocated, so the first SetProp
// is what builds the map.
func TestObject(t *testing.T) {
	t.Parallel()
	class := &Class{Name: "Invoice", Fields: []Field{{Name: "total"}}}
	obj := NewObject(class)

	if obj.Class != class {
		t.Error("NewObject lost the class it was handed")
	}
	if obj.Len() != 0 {
		t.Errorf("a fresh object has %d properties, want 0", obj.Len())
	}
	if _, ok := obj.Prop("total"); ok {
		t.Error(`Prop("total") answered true on a declared property nothing has set`)
	}

	obj.SetProp("total", int64(42))
	value, ok := obj.Prop("total")
	if !ok {
		t.Fatal(`Prop("total") = false after SetProp`)
	}
	if value != int64(42) {
		t.Errorf(`Prop("total") = %v, want 42`, value)
	}
	if obj.Len() != 1 {
		t.Errorf("Len() = %d after one SetProp, want 1", obj.Len())
	}
}

// TestObject_SetID covers the name a constructed object records. The runtime
// writes the receiving variable there, so a value printed back names its
// assignment site.
func TestObject_SetID(t *testing.T) {
	t.Parallel()
	obj := NewObject(nil)
	if obj.ID != "" {
		t.Errorf("a fresh object carries the ID %q", obj.ID)
	}

	obj.SetID("invoice")
	if obj.ID != "invoice" {
		t.Errorf("ID = %q after SetID, want \"invoice\"", obj.ID)
	}

	obj.SetID("other")
	if obj.ID != "other" {
		t.Errorf("ID = %q after a second SetID, want \"other\"", obj.ID)
	}
}

// TestObject_Names covers the order PHP reads properties back in: the declared
// fields in declaration order, then what the script added, in the order it
// added it. json_encode, print_r, the (array) cast and foreach all read this.
//
// A property written straight into Props bypasses the record and sorts after
// the rest, so a missed SetProp degrades to a stable order and never a random
// one.
func TestObject_Names(t *testing.T) {
	t.Parallel()
	class := &Class{Fields: []Field{{Name: "total"}, {Name: "currency"}}}
	obj := NewObject(class)

	obj.SetProp("dynamic", 1)
	obj.SetProp("currency", "EUR")
	obj.SetProp("total", int64(42))

	if got, want := obj.Names(), []string{"total", "currency", "dynamic"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}

	obj.Props["unrecorded"] = true
	if got, want := obj.Names(), []string{"total", "currency", "dynamic", "unrecorded"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v with a direct Props write, want %v", got, want)
	}
}

// TestObject_DeleteProp covers unset on a property. A declared field assigned
// again lands back in its declared position, and SetProp appends a dynamic one,
// so the order survives the round trip the way PHP's does.
func TestObject_DeleteProp(t *testing.T) {
	t.Parallel()
	class := &Class{Fields: []Field{{Name: "total"}}}
	obj := NewObject(class)
	obj.SetProp("total", int64(1))
	obj.SetProp("note", "first")

	obj.DeleteProp("total")
	if _, ok := obj.Prop("total"); ok {
		t.Error("the property is still readable after DeleteProp")
	}
	if got, want := obj.Names(), []string{"note"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v after DeleteProp, want %v", got, want)
	}

	obj.SetProp("total", int64(2))
	if got, want := obj.Names(), []string{"total", "note"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v after reassigning a declared field, want %v", got, want)
	}

	before := obj.Len()
	obj.DeleteProp("absent")
	if obj.Len() != before {
		t.Errorf("DeleteProp on an absent name changed Len() from %d to %d", before, obj.Len())
	}
}

// TestObject_Range covers walking the properties in Names order, and stopping
// where fn answers false.
func TestObject_Range(t *testing.T) {
	t.Parallel()
	obj := NewObject(&Class{Fields: []Field{{Name: "a"}, {Name: "b"}, {Name: "c"}}})
	obj.SetProp("a", 1)
	obj.SetProp("b", 2)
	obj.SetProp("c", 3)

	var seen []string
	obj.Range(func(name string, _ any) bool {
		seen = append(seen, name)
		return true
	})
	if got, want := seen, []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Range visited %v, want %v", got, want)
	}

	seen = nil
	obj.Range(func(name string, _ any) bool {
		seen = append(seen, name)
		return name != "b"
	})
	if got, want := seen, []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Range visited %v after fn answered false, want %v", got, want)
	}
}
