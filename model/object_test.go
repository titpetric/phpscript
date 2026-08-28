package model_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

func names(o *model.Object) string {
	return strings.Join(o.Names(), ",")
}

// A property reads back where it was added, which is what PHP does and what
// json_encode, print_r, var_dump, get_object_vars, the (array) cast and foreach
// all show a script.
func TestObjectKeepsInsertionOrder(t *testing.T) {
	o := model.NewStdClass()
	o.SetProp("c", 3)
	o.SetProp("a", 1)
	o.SetProp("b", 2)

	if got := names(o); got != "c,a,b" {
		t.Errorf("Names() = %q, want the order they were set", got)
	}
	// Assigning over a property that is set does not move it.
	o.SetProp("c", 30)
	if got := names(o); got != "c,a,b" {
		t.Errorf("Names() after reassigning = %q, want the order unchanged", got)
	}
	if v, ok := o.Prop("c"); !ok || v != 30 {
		t.Errorf("Prop(c) = %v, %v, want 30, true", v, ok)
	}
	if o.Len() != 3 {
		t.Errorf("Len() = %d, want 3", o.Len())
	}
}

// A declared field holds the slot its declaration gave it: it comes first
// whatever order the properties were assigned in, and returns to that slot
// after an unset. A dynamic property has no slot and moves to the end.
func TestObjectOrdersDeclaredFieldsFirst(t *testing.T) {
	class := &model.Class{Name: "Row", Fields: []model.Field{{Name: "id"}, {Name: "name"}}}
	o := model.NewObject(class)
	o.SetProp("id", 1)
	o.SetProp("name", "first")
	o.SetProp("zeta", "z")
	o.SetProp("alpha", "a")

	if got := names(o); got != "id,name,zeta,alpha" {
		t.Fatalf("Names() = %q, want declared fields then the added ones", got)
	}

	o.DeleteProp("id")
	if got := names(o); got != "name,zeta,alpha" {
		t.Errorf("Names() after unset = %q, want id gone", got)
	}
	o.SetProp("id", 9)
	if got := names(o); got != "id,name,zeta,alpha" {
		t.Errorf("Names() after re-adding a declared field = %q, want its declared slot back", got)
	}

	o.DeleteProp("zeta")
	o.SetProp("zeta", "Z")
	if got := names(o); got != "id,name,alpha,zeta" {
		t.Errorf("Names() after re-adding a dynamic property = %q, want it at the end", got)
	}
}

// A property written straight into the map, bypassing SetProp, is still
// reported rather than lost, so a call site that misses the accessor degrades
// to a stable order instead of dropping data.
func TestObjectReportsUnrecordedProperties(t *testing.T) {
	o := model.NewStdClass()
	o.SetProp("first", 1)
	o.Props["zulu"] = 2
	o.Props["alpha"] = 3

	if got := names(o); got != "first,alpha,zulu" {
		t.Errorf("Names() = %q, want the recorded one then the rest sorted", got)
	}

	seen := 0
	o.Range(func(string, any) bool {
		seen++
		return true
	})
	if seen != 3 {
		t.Errorf("Range visited %d properties, want 3", seen)
	}
}

// Deleting a property that was never set leaves the object alone, and Range
// stops when its callback says to.
func TestObjectDeleteMissingAndRangeStop(t *testing.T) {
	o := model.NewStdClass()
	o.SetProp("a", 1)
	o.SetProp("b", 2)
	o.DeleteProp("missing")
	if got := names(o); got != "a,b" {
		t.Errorf("Names() = %q, want a,b", got)
	}

	visited := ""
	o.Range(func(name string, _ any) bool {
		visited += name
		return false
	})
	if visited != "a" {
		t.Errorf("Range visited %q, want it to stop after the first", visited)
	}
}
