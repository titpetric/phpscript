package runner

import (
	"testing"
)

// TestAssignGoIndex covers the writes a script makes into a binding's native
// Go collection: a map accepts new and replacement keys, a slice element
// takes the write through the shared backing array, and any other target
// answers an error.
func TestAssignGoIndex(t *testing.T) {
	t.Parallel()
	set := func(v any) func(any) (any, error) {
		return func(any) (any, error) { return v, nil }
	}

	counts := map[string]int{"n": 1}
	if err := assignGoIndex(counts, "n", set(int64(7))); err != nil {
		t.Fatalf("assignGoIndex on a map failed: %v", err)
	}
	if counts["n"] != 7 {
		t.Errorf("the map holds %d, want the written value", counts["n"])
	}
	if err := assignGoIndex(counts, "added", set(int64(2))); err != nil {
		t.Fatalf("a new map key failed: %v", err)
	}
	if counts["added"] != 2 {
		t.Errorf("the map holds %d under the new key, want 2", counts["added"])
	}

	names := []string{"a", "b"}
	if err := assignGoIndex(names, int64(1), set("edited")); err != nil {
		t.Fatalf("assignGoIndex on a slice failed: %v", err)
	}
	if names[1] != "edited" {
		t.Errorf("the slice holds %q, want the written value", names[1])
	}
	if err := assignGoIndex(names, int64(5), set("lost")); err == nil {
		t.Error("an out-of-range slice index reported no error")
	}

	var nilMap map[string]int
	if err := assignGoIndex(nilMap, "n", set(int64(1))); err == nil {
		t.Error("a write to a nil map reported no error")
	}
	if err := assignGoIndex("text", int64(0), set("x")); err == nil {
		t.Error("a write into a scalar reported no error")
	}
}

// TestUnsetGoIndex covers unset($x[$k]) over a binding's Go collection. A map
// deletes in place and answers nothing; a slice cannot hold a hole, so it
// answers a reslice around the element for the caller to assign back; a miss
// changes nothing, so the unset runs unconditionally.
func TestUnsetGoIndex(t *testing.T) {
	t.Parallel()
	counts := map[string]int{"n": 1, "m": 2}
	if replacement, changed := unsetGoIndex(counts, "n"); changed || replacement != nil {
		t.Errorf("a map delete answered (%#v, %v), want in place", replacement, changed)
	}
	if _, held := counts["n"]; held {
		t.Error("the map still holds the deleted key")
	}

	names := []string{"a", "b", "c"}
	replacement, changed := unsetGoIndex(names, int64(1))
	if !changed {
		t.Fatal("a slice element removal reported no replacement")
	}
	shorter, ok := replacement.([]string)
	if !ok || len(shorter) != 2 || shorter[0] != "a" || shorter[1] != "c" {
		t.Errorf("the reslice = %#v, want the element removed and the rest renumbered", replacement)
	}

	if _, changed := unsetGoIndex(names, int64(9)); changed {
		t.Error("an out-of-range index reported a change")
	}
	if _, changed := unsetGoIndex("text", int64(0)); changed {
		t.Error("a scalar reported a change")
	}
}

// TestAssignGoField covers `$rec->is_readonly = true` landing on a Go struct:
// the name resolves through the fold ladder, the value coerces to the field's
// type, and an unwritable or missing field answers an error, with no panic.
func TestAssignGoField(t *testing.T) {
	t.Parallel()
	type record struct {
		Value      string
		IsReadonly bool
	}
	rec := &record{Value: "before"}

	if err := assignGoField(rec, "value", func(any) (any, error) { return "after", nil }); err != nil {
		t.Fatalf("assignGoField failed: %v", err)
	}
	if rec.Value != "after" {
		t.Errorf("the field holds %q, want the written value", rec.Value)
	}

	// The snake_case spelling resolves the idiomatic Go name, and the value
	// takes the field's type the way a parameter does.
	if err := assignGoField(rec, "is_readonly", func(any) (any, error) { return true, nil }); err != nil {
		t.Fatalf("the folded spelling failed: %v", err)
	}
	if !rec.IsReadonly {
		t.Error("the folded field was not written")
	}

	if err := assignGoField(rec, "absent", func(any) (any, error) { return 1, nil }); err == nil {
		t.Error("a missing field reported no error")
	}
	if err := assignGoField(record{}, "value", func(any) (any, error) { return "x", nil }); err == nil {
		t.Error("an unaddressable struct value reported no error")
	}
	if err := assignGoField(nil, "value", func(any) (any, error) { return "x", nil }); err == nil {
		t.Error("a nil base reported no error")
	}
	if err := assignGoField(rec, "is_readonly", func(any) (any, error) { return int64(1), nil }); err == nil {
		t.Error("an inconvertible value reported no error")
	}
}
