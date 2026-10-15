package stdlib_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/titpetric/phpscript/stdlib"
)

// TestException covers the one Go type behind every PHP throwable. Class
// records the name the script constructed, so get_class() can answer it and a
// catch clause can filter on it, and no two of them stand in a subclass
// relation: phpscript has no inheritance. The constructor returns a pointer,
// so the instance a script holds is addressable and `$e->message = "..."`
// writes to it; the nil error is what keeps `new Exception` from throwing at
// the point it is constructed.
func TestException(t *testing.T) {
	t.Parallel()
	exception, err := stdlib.NewException("boom", 7)
	if err != nil {
		t.Fatalf("NewException() = %v, want no error from constructing one", err)
	}
	if exception.Message != "boom" || exception.Code != 7 || exception.Class != "Exception" {
		t.Fatalf("exception = %+v", exception)
	}

	// A script writing a field mutates the instance it holds, so the accessors
	// read the write back.
	exception.Message = "changed"
	exception.Code = 404
	if exception.GetMessage() != "changed" || exception.GetCode() != 404 {
		t.Errorf("after a field write, exception = %+v", exception)
	}
}

// TestException_ThrowableClass covers the name a catch clause filters on. An
// exception carrying no class is PHP's base Exception, so a value built as a
// zero struct is still catchable; the SPL and Error class names are the same
// Go type with a different name recorded, so each answers its own.
func TestException_ThrowableClass(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		class string
		want  string
	}{
		{name: "no class recorded", want: "Exception"},
		{name: "base", class: "Exception", want: "Exception"},
		{name: "SPL", class: "InvalidArgumentException", want: "InvalidArgumentException"},
		{name: "error", class: "DivisionByZeroError", want: "DivisionByZeroError"},
	} {
		t.Run(test.name, func(t *testing.T) {
			exception := &stdlib.Exception{Message: "boom", Class: test.class}
			if got := exception.ThrowableClass(); got != test.want {
				t.Errorf("ThrowableClass() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestException_GetCode covers the accessor behind getCode(). PHP's default
// code is 0, so an exception constructed with no code answers zero and a
// script comparing against it needs no null check.
func TestException_GetCode(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		code int
		want int
	}{
		{name: "default", want: 0},
		{name: "recorded", code: 7, want: 7},
		{name: "negative", code: -1, want: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			exception, err := stdlib.NewException("boom", test.code)
			if err != nil {
				t.Fatal(err)
			}
			if got := exception.GetCode(); got != test.want {
				t.Errorf("GetCode() = %d, want %d", got, test.want)
			}
		})
	}
}

// TestException_GetMessage covers the accessor behind getMessage(). It reads
// the field, so a message a script replaced is the one that comes back, and an
// exception constructed with no message answers an empty string.
func TestException_GetMessage(t *testing.T) {
	t.Parallel()
	exception, err := stdlib.NewException("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := exception.GetMessage(); got != "" {
		t.Errorf("GetMessage() = %q, want an empty string", got)
	}

	exception.Message = "connection refused"
	if got := exception.GetMessage(); got != "connection refused" {
		t.Errorf("GetMessage() = %q, want the message the script wrote", got)
	}
}

// TestException_Error covers the error interface. A thrown exception reaching a
// Go caller is an ordinary error carrying the message alone, so a host wraps it
// with %w and errors.As finds it again: the class the script named is on the
// value and never in the text.
func TestException_Error(t *testing.T) {
	t.Parallel()
	exception := &stdlib.Exception{Message: "connection refused", Code: 7, Class: "RuntimeException"}

	var err error = exception
	if got := err.Error(); got != "connection refused" {
		t.Errorf("Error() = %q, want the message alone", got)
	}

	wrapped := fmt.Errorf("serving the request: %w", err)
	var found *stdlib.Exception
	if !errors.As(wrapped, &found) {
		t.Fatal("errors.As did not find the Exception through a wrapper")
	}
	if found.ThrowableClass() != "RuntimeException" || found.GetCode() != 7 {
		t.Errorf("the unwrapped exception = %+v", found)
	}
}
