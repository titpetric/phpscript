package runner

import (
	"errors"
	"testing"
)

// TestThrowable covers the predicate that separates a PHP throwable from an
// error a Go binding returned. The class is read off the value, because every
// SPL name is one Go type here: reflection over the Go type would report
// "Exception" for an InvalidArgumentException.
func TestThrowable(t *testing.T) {
	t.Parallel()
	var thrown Throwable = &TypeError{Name: "str_repeat", Position: 1, Want: "int", Got: "string"}
	if got, want := thrown.ThrowableClass(), "TypeError"; got != want {
		t.Errorf("ThrowableClass() = %q, want %q", got, want)
	}
	if thrown.Error() == "" {
		t.Error("a Throwable rendered an empty message")
	}
}

// TestThrowableClassOf covers routing a catch clause. A value naming its class
// reports the name, an error from a Go binding reports no class, and so does a
// throwable whose name is blank: matchCatchType reads the false and lets any
// clause take the error, which is the contract a host binding is written
// against.
func TestThrowableClassOf(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		err   error
		class string
		ok    bool
	}{
		{"a named throwable", &ArgumentCountError{Name: "substr"}, "ArgumentCountError", true},
		{"a blank throwable", &blankThrowable{}, "", false},
		{"a binding's error", errors.New("no such table"), "", false},
	} {
		class, ok := throwableClassOf(tc.err)
		if ok != tc.ok {
			t.Errorf("%s: throwableClassOf() = %v, want %v", tc.name, ok, tc.ok)
		}
		if class != tc.class {
			t.Errorf("%s: class = %q, want %q", tc.name, class, tc.class)
		}
	}
}

// blankThrowable implements the interface with no class name, the shape a
// driver type called Error takes when something reads it as PHP's Error class.
type blankThrowable struct{}

func (blankThrowable) Error() string          { return "driver failure" }
func (blankThrowable) ThrowableClass() string { return "" }
