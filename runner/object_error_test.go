package runner

import (
	"errors"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestObjectError covers the wrapper a thrown instance travels in. An object is
// not an error; it needs a wrapper to take the path a built-in throwable takes.
// The recorded class both filters a catch clause and answers get_class(). A
// class declaring `extends` stays a single class here.
func TestObjectError(t *testing.T) {
	t.Parallel()
	obj := model.NewObject(&model.Class{Name: "AppException"})
	obj.SetProp("message", "no such invoice")

	err := newObjectError(obj)
	if got, want := err.Error(), "AppException: no such invoice"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got, want := err.ThrowableClass(), "AppException"; got != want {
		t.Errorf("ThrowableClass() = %q, want %q", got, want)
	}
}

// TestObjectErrorRendering covers the renderings an uncaught throw falls back
// to. A class without a message prints the class; an instance of no class prints
// "object"; a wrapper around nothing still renders a message. A nil class
// reports no name, and matchCatchType routes it as it routes an error a binding
// returned.
func TestObjectErrorRendering(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		object *model.Object
		want   string
		class  string
	}{
		{
			name:   "a class with no message",
			object: model.NewObject(&model.Class{Name: "AppException"}),
			want:   "uncaught exception: AppException",
			class:  "AppException",
		},
		{
			name:   "an instance of no class",
			object: model.NewObject(nil),
			want:   "uncaught exception: object",
		},
		{
			name: "no object",
			want: "uncaught exception",
		},
	} {
		err := newObjectError(tc.object)
		if got := err.Error(); got != tc.want {
			t.Errorf("%s: Error() = %q, want %q", tc.name, got, tc.want)
		}
		if got := err.ThrowableClass(); got != tc.class {
			t.Errorf("%s: ThrowableClass() = %q, want %q", tc.name, got, tc.class)
		}
	}
}

// TestCatchValue covers the value a catch clause binds. A script that threw an
// instance gets the instance back. It does not get the wrapper. Everything else
// binds the error itself; a Go binding's failure reaches `catch` as a value a
// script can read a message from.
func TestCatchValue(t *testing.T) {
	t.Parallel()
	obj := model.NewObject(&model.Class{Name: "AppException"})
	if got := catchValue(newObjectError(obj)); got != any(obj) {
		t.Errorf("catchValue() = %#v, want the thrown object", got)
	}

	plain := errors.New("no such table")
	if got := catchValue(plain); got != any(plain) {
		t.Errorf("catchValue() = %#v, want the error itself", got)
	}
}
