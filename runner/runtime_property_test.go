package runner_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// propertyRecord is a Go value a binding returns to a script, the shape
// PropertyExists answers for by reflection.
type propertyRecord struct {
	ID        int64
	CreatedAt string
	hidden    string
}

// TestPropertyExists covers the two kinds of object a script holds: the
// interpreted one, whose class declaration outlives an unset, and the Go value
// a binding returned, whose fields resolve the way a property read resolves
// them.
func TestPropertyExists(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})

	class := &model.Class{Name: "Record", Fields: []model.Field{{Name: "id"}}}
	object := model.NewObject(class)
	object.SetProp("name", "alpha")

	record := propertyRecord{ID: 1, CreatedAt: "now", hidden: "no"}

	cases := []struct {
		name     string
		value    any
		property string
		want     bool
	}{
		{"a property the object holds", object, "name", true},
		{"a property the class declares", object, "id", true},
		{"a property neither names", object, "absent", false},
		{"property names are case sensitive", object, "Name", false},
		{"an object with no class", model.NewStdClass(), "absent", false},
		{"an exported Go field", record, "ID", true},
		{"a Go field case-insensitively", record, "id", true},
		{"a Go field without the underscores", &record, "created_at", true},
		{"an unexported Go field is no property", record, "hidden", false},
		{"a Go field no struct carries", record, "absent", false},
		{"a scalar carries no property", "alpha", "length", false},
		{"null carries no property", nil, "anything", false},
		{"an array carries no property", model.NewArray(), "0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rt.PropertyExists(tc.value, tc.property); got != tc.want {
				t.Errorf("PropertyExists(%T, %q) is %v, want %v", tc.value, tc.property, got, tc.want)
			}
		})
	}
}
