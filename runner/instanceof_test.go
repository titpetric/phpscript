package runner

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestPHPInstanceOf covers `$value instanceof Class` as class-name equality,
// compared without case as class names are everywhere else. A class declaring
// `extends RuntimeException` is an instance of itself and of nothing else,
// because there is no inheritance to follow; an interface name matches against
// the list the declaration recorded. See docs/design.md.
func TestPHPInstanceOf(t *testing.T) {
	t.Parallel()
	store := model.NewObject(&model.Class{Name: "Store", Implements: []string{"Reader", "Writer"}})
	derived := model.NewObject(&model.Class{Name: "AppException"})

	for _, tc := range []struct {
		name  string
		value any
		class any
		want  bool
	}{
		{"its own class", store, "Store", true},
		{"its own class, other case", store, "store", true},
		{"a leading separator", store, `\Store`, true},
		{"a declared interface", store, "Reader", true},
		{"a second declared interface", store, "Writer", true},
		{"an interface nothing declared", store, "Countable", false},
		{"a class it extends", derived, "RuntimeException", false},
		{"Throwable", derived, "Throwable", false},
		{"an integer", int64(1), "Store", false},
		{"null", nil, "Store", false},
		{"an array", model.NewArray(), "Store", false},
		{"the class held in a variable", store, "Store", true},
		{"the class taken off an instance", store, model.NewObject(store.Class), true},
		{"no class named", store, nil, false},
	} {
		if got := phpInstanceOf(tc.value, tc.class); got != tc.want {
			t.Errorf("%s: phpInstanceOf() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestInstanceOfClass covers the class name a value reports. A scalar is an
// instance of nothing, so `1 instanceof Foo` is false and raises nothing. A
// callable value is php's Closure whichever of the three spellings it holds,
// and a host-backed value reports its Go type name, so `new Database("app")`
// answers "Database".
func TestInstanceOfClass(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"a declared class", model.NewObject(&model.Class{Name: "Store"}), "Store"},
		{"an instance of no class", model.NewObject(nil), ""},
		{"an integer", int64(1), ""},
		{"a string", "Store", ""},
		{"a bool", true, ""},
		{"null", nil, ""},
		{"an array", model.NewArray(), ""},
		{"a callable", &Callable{}, "Closure"},
		{"a throwable", &TypeError{}, "TypeError"},
		{"a host value", &Scope{}, "Scope"},
	} {
		if got := instanceOfClass(tc.value); got != tc.want {
			t.Errorf("%s: instanceOfClass() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestPHPClassName covers the name get_class() reports: the declared name for
// an interpreted object, the Go type name for a host-backed one, and
// get_debug_type()'s spelling for a value of no class at all.
func TestPHPClassName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"a declared class", model.NewObject(&model.Class{Name: "Store"}), "Store"},
		{"a host value", &Scope{}, "Scope"},
		{"an integer", int64(1), "int"},
		{"a string", "text", "string"},
		{"null", nil, "null"},
		{"an array", model.NewArray(), "array"},
	} {
		if got := phpClassName(tc.value); got != tc.want {
			t.Errorf("%s: phpClassName() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestGoTypeName covers the declared name of the Go type behind a value, read
// through any depth of pointer, which is the class name a script sees for a
// host-backed object. An unnamed type reports nothing, and so does nil.
func TestGoTypeName(t *testing.T) {
	t.Parallel()
	scope := &Scope{}
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"a pointer to a struct", scope, "Scope"},
		{"a pointer to a pointer", &scope, "Scope"},
		{"a named scalar", Size(1), "Size"},
		{"an unnamed type", []string{"a"}, ""},
		{"nil", nil, ""},
	} {
		if got := goTypeName(tc.value); got != tc.want {
			t.Errorf("%s: goTypeName() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
