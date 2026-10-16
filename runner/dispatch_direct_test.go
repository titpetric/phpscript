package runner

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

// directTestWriter answers Write and WriteString, standing in for the
// response writer the typed bridge serves.
type directTestWriter struct {
	strings.Builder
	stringWrites int
}

func (w *directTestWriter) WriteString(s string) (int, error) {
	w.stringWrites++
	return w.Builder.WriteString(s)
}

// TestDirectGoMethodMatchesTheReflectPath holds the typed bridge to the one
// rule it has: it answers exactly as the planned reflect call answers, for
// the value, the arity error and the fall-through alike.
func TestDirectGoMethodMatchesTheReflectPath(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})
	scope := func() *Scope { return NewScope() }

	request := httptest.NewRequest("GET", "/users/42?name=tit", nil)
	request.SetPathValue("id", "42")

	got, err := rt.callGoMethod(request, "path_value", []any{"id"}, scope)
	if err != nil || got != "42" {
		t.Errorf("path_value = %v, %v, want the path segment", got, err)
	}
	got, err = rt.callGoMethod(request.URL.Query(), "get", []any{"name"}, scope)
	if err != nil || got != "tit" {
		t.Errorf("query get = %v, %v, want the parameter", got, err)
	}
	// A missing argument pads to the zero value, as the reflect path pads.
	got, err = rt.callGoMethod(request, "path_value", nil, scope)
	if err != nil || got != "" {
		t.Errorf("path_value() = %v, %v, want the empty string", got, err)
	}
	// One argument too many is the same ArgumentCountError, named.
	if _, err = rt.callGoMethod(request, "path_value", []any{"id", "extra"}, scope); err == nil {
		t.Error("an extra argument raised nothing")
	} else {
		var count *ArgumentCountError
		if !errors.As(err, &count) || count.Name != "path_value" {
			t.Errorf("extra argument raised %v, want a named ArgumentCountError", err)
		}
	}

	writer := &directTestWriter{}
	got, err = rt.callGoMethod(writer, "write", []any{"hello"}, scope)
	if err != nil || got != 5 {
		t.Errorf("write = %v (%T), %v, want the count the writer reported", got, got, err)
	}
	if writer.stringWrites != 1 {
		t.Errorf("stringWrites = %d, want the write through io.StringWriter", writer.stringWrites)
	}
	// An argument the bridge does not cover takes the reflect path: an int64
	// converts to no []byte, and the TypeError is that path's own.
	if _, err = rt.callGoMethod(writer, "write", []any{int64(7)}, scope); err == nil {
		t.Error("write(7) raised nothing")
	} else {
		var mismatch *TypeError
		if !errors.As(err, &mismatch) {
			t.Errorf("write(7) raised %v, want the reflect path's TypeError", err)
		}
	}
}
