package runner

import (
	"errors"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestObjectError covers the wrapper a thrown instance travels in. An object is
// not an error, so it needs one to take the path a built-in throwable takes,
// and the class it records is both what a catch clause filters on and what
// get_class() reports. A class declaring `extends` is still only its own class
// here.
func TestObjectError(t *testing.T) {
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
// to: a class with no message prints the class, an instance of no class prints
// "object", and a wrapper around nothing still renders a message. A nil class
// reports no name at all, so matchCatchType routes it the way it routes an
// error a binding returned.
func TestObjectErrorRendering(t *testing.T) {
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

// TestCatchValue covers what a catch clause binds. A script that threw an
// instance gets the instance back, never the wrapper; everything else binds the
// error itself, so a Go binding's failure reaches `catch` as a value a script
// can read a message off.
func TestCatchValue(t *testing.T) {
	obj := model.NewObject(&model.Class{Name: "AppException"})
	if got := catchValue(newObjectError(obj)); got != any(obj) {
		t.Errorf("catchValue() = %#v, want the thrown object", got)
	}

	plain := errors.New("no such table")
	if got := catchValue(plain); got != any(plain) {
		t.Errorf("catchValue() = %#v, want the error itself", got)
	}
}
