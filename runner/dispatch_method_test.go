package runner

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/titpetric/phpscript/model"
)

// TestMethodCallError covers the same naming for a method call. bindGoMethod
// reports nameless, because the cache is per receiver type and the spelled name
// belongs to the call site.
func TestMethodCallError(t *testing.T) {
	t.Parallel()
	arity := &ArgumentCountError{Want: 1, Got: 2}
	if got := methodCallError(arity, "get"); got != error(arity) || arity.Name != "get" {
		t.Errorf("the arity error names %q, want %q", arity.Name, "get")
	}

	mismatch := &TypeError{Position: 1, Want: "int", Got: "string"}
	if got := methodCallError(mismatch, "get"); got != error(mismatch) || mismatch.Name != "get" {
		t.Errorf("the type error names %q, want %q", mismatch.Name, "get")
	}

	other := errors.New("no such table")
	if got := methodCallError(other, "get"); !errors.Is(got, other) {
		t.Errorf("methodCallError() = %v on an unrelated error, want the error", got)
	}
}

// TestThrowableMethod covers PHP's Throwable method set over any Go error and a
// catch clause binds whatever reached it: a thrown Exception, an error a
// binding returned, a converted panic. A method the concrete Go type defines
// wins and *stdlib.Exception reports its own code.
func TestThrowableMethod(t *testing.T) {
	t.Parallel()
	inner := errors.New("no such table")
	err := &HostPanicError{Callable: "Storage::open", Value: "nil map"}

	for _, tc := range []struct {
		method string
		want   any
	}{
		{"getMessage", err.Error()},
		{"get_message", err.Error()},
		{"__toString", err.Error()},
		{"getCode", int64(0)},
		{"getFile", ""},
		{"getLine", int64(0)},
		{"getTraceAsString", "#0 {main}"},
	} {
		value, ok := throwableMethod(err, tc.method)
		if !ok {
			t.Errorf("throwableMethod(%q) = false", tc.method)
			continue
		}
		if value != tc.want {
			t.Errorf("throwableMethod(%q) = %#v, want %#v", tc.method, value, tc.want)
		}
	}

	if _, ok := throwableMethod(err, "query"); ok {
		t.Error("throwableMethod() answered a method Throwable does not declare")
	}
	if _, ok := throwableMethod("text", "getMessage"); ok {
		t.Error("throwableMethod() answered for a value that is not an error")
	}

	previous, ok := throwableMethod(fmt.Errorf("while opening: %w", inner), "getPrevious")
	if !ok {
		t.Fatal("getPrevious reported false")
	}
	wrapped, ok := previous.(error)
	if !ok {
		t.Fatalf("getPrevious = %#v, want an error", previous)
	}
	if !errors.Is(wrapped, inner) {
		t.Errorf("getPrevious = %v, want the wrapped error", wrapped)
	}
	if value, _ := throwableMethod(inner, "getPrevious"); value != nil {
		t.Errorf("getPrevious on an unwrapped error = %#v, want nil", value)
	}

	trace, _ := throwableMethod(err, "getTrace")
	if array, ok := trace.(*model.Array); !ok || array.Len() != 0 {
		t.Errorf("getTrace = %#v, want an empty array", trace)
	}
}

// TestMethodByNameFold covers PHP's case-insensitive method resolution, then
// the retry without underscores and a snake_case script name finds the
// idiomatic Go one: `$t->iso_week()` resolves ISOWeek.
func TestMethodByNameFold(t *testing.T) {
	t.Parallel()
	value := reflect.ValueOf(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	for _, name := range []string{"ISOWeek", "isoweek", "iso_week"} {
		if method := methodByNameFold(value, name); !method.IsValid() {
			t.Errorf("methodByNameFold(%q) found nothing", name)
		}
	}
	if method := methodByNameFold(value, "no_such_method"); method.IsValid() {
		t.Error("methodByNameFold() found a method that does not exist")
	}
}

// TestFieldByNameFold covers property access on a Go struct: the exact name
// first, then without case, then without underscores and `$rec->is_readonly`
// reads the field IsReadonly.
func TestFieldByNameFold(t *testing.T) {
	t.Parallel()
	type record struct {
		Value      string
		IsReadonly bool
	}
	value := reflect.ValueOf(record{Value: "text", IsReadonly: true})

	for _, name := range []string{"Value", "value", "VALUE"} {
		field := fieldByNameFold(value, name)
		if !field.IsValid() {
			t.Errorf("fieldByNameFold(%q) found nothing", name)
			continue
		}
		if field.String() != "text" {
			t.Errorf("fieldByNameFold(%q) = %q, want %q", name, field.String(), "text")
		}
	}

	folded := fieldByNameFold(value, "is_readonly")
	if !folded.IsValid() || !folded.Bool() {
		t.Error("fieldByNameFold() did not resolve the snake_case spelling")
	}
	if field := fieldByNameFold(value, "absent"); field.IsValid() {
		t.Error("fieldByNameFold() found a field that does not exist")
	}
}
