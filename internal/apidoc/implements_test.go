package apidoc

import (
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// TestReflectImplements covers the clause a class definition prints. The set
// is the one interface of this module a script names and the standard library
// conventions a caller acts on, and a bound type satisfying none of it prints
// no clause.
func TestReflectImplements(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ  reflect.Type
		want string
	}{
		// A throwable cites Throwable alone. runner.Throwable embeds
		// error, and naming both would say it twice.
		{reflect.TypeOf(runner.NewRuntimeException("boom", 0)), "Throwable"},
		{reflect.TypeOf(http.NewServeMux()), "http.Handler"},
		{reflect.TypeOf(time.Time{}), "encoding.TextMarshaler, fmt.Stringer, json.Marshaler"},
		{reflect.TypeOf(time.Duration(0)), "fmt.Stringer"},
		{reflect.TypeOf(regexp.MustCompile("x")), "encoding.TextMarshaler, encoding.TextUnmarshaler, fmt.Stringer"},
		// HTTP\Client binds this type and satisfies nothing in the set.
		{reflect.TypeOf(&http.Client{}), ""},
	}
	for _, c := range cases {
		names := []string{}
		for _, cite := range reflectImplements(c.typ) {
			names = append(names, cite.Clause())
		}
		if got := strings.Join(names, ", "); got != c.want {
			t.Errorf("reflectImplements(%s) = %q, want %q", c.typ, got, c.want)
		}
	}
}

// TestReflectImplementsCites pins how the listing cites a matched interface: a
// standard library one by import path with its reference page; this module's
// by name, because no page exists to point at.
func TestReflectImplementsCites(t *testing.T) {
	t.Parallel()
	mux := reflectImplements(reflect.TypeOf(http.NewServeMux()))
	if len(mux) != 1 {
		t.Fatalf("ServeMux cites %d interfaces, want 1: %+v", len(mux), mux)
	}
	if got, want := mux[0].Link(), "[net/http.Handler](https://pkg.go.dev/net/http#Handler)"; got != want {
		t.Errorf("standard library citation = %q, want %q", got, want)
	}

	thrown := reflectImplements(reflect.TypeOf(runner.NewRuntimeException("boom", 0)))
	if len(thrown) != 1 {
		t.Fatalf("RuntimeException cites %d interfaces, want 1: %+v", len(thrown), thrown)
	}
	if got, want := thrown[0].Link(), "github.com/titpetric/phpscript/runner.Throwable"; got != want {
		t.Errorf("module citation = %q, want the bare symbol %q", got, want)
	}
	// The clause prefers the PHP spelling, because Throwable is what a catch
	// clause and instanceof name, and the Go symbol stays the citation.
	if got, want := thrown[0].Clause(), "Throwable"; got != want {
		t.Errorf("clause = %q, want %q", got, want)
	}
}

// TestCovered covers the deduplication. The clause drops an interface another
// match implies, and keeps two unrelated matches.
func TestCovered(t *testing.T) {
	t.Parallel()
	throwable := reflect.TypeOf((*runner.Throwable)(nil)).Elem()
	failure := reflect.TypeOf((*error)(nil)).Elem()
	matched := []reflect.Type{throwable, failure}

	if !covered(failure, matched) {
		t.Error("error is not reported as covered by Throwable, which embeds it")
	}
	if covered(throwable, matched) {
		t.Error("Throwable is reported as covered, and nothing in the set implies it")
	}
}
