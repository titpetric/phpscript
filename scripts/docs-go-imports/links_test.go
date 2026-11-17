package main

import (
	"testing"
)

// TestURL covers the link for a row: a symbol the package declares, a helper
// the runtime adds beside it, and a class resolving through the Go type its
// wrapper holds.
func TestURL(t *testing.T) {
	t.Parallel()
	g := newGenerator()
	for _, tc := range []struct {
		area, kind, name string
		want             string
	}{
		{`Runtime\Go\Net\HTTP`, "function", `Runtime\Go\Net\HTTP\StatusText`, "https://pkg.go.dev/net/http#StatusText"},
		{`Runtime\Go\Net\HTTP`, "constant", `Runtime\Go\Net\HTTP\StatusOK`, "https://pkg.go.dev/net/http#StatusOK"},
		{`Runtime\Go\Net\HTTP`, "class", `Runtime\Go\Net\HTTP\Mux`, "https://pkg.go.dev/net/http#ServeMux"},
		{`Runtime\Go\Net\HTTP`, "class", `Runtime\Go\Net\HTTP\Request`, "https://pkg.go.dev/net/http#Request"},
		{`Runtime\Go\Net\HTTP`, "method", `Runtime\Go\Net\HTTP\Mux::handle`, "https://pkg.go.dev/net/http#ServeMux.Handle"},
		{`Runtime\Go\Net\HTTP`, "method", `Runtime\Go\Net\HTTP\ServeMux::serve_http`, "https://pkg.go.dev/net/http#ServeMux.ServeHTTP"},
		{`Runtime\Go\Database\SQL`, "constant", `Runtime\Go\Database\SQL\ErrNoRows`, "https://pkg.go.dev/database/sql#ErrNoRows"},
		// A name the runtime adds beside the package has no page to link.
		{`Runtime\Go\Net\HTTP`, "function", `Runtime\Go\Net\HTTP\Aborted`, ""},
		{`Runtime\Go\Net\HTTP`, "function", `Runtime\Go\Net\HTTP\IgnoreUserAbort`, ""},
		{`Runtime\Go\Net\HTTP`, "method", `Runtime\Go\Net\HTTP\Mux::use`, ""},
		// The builtins answer under the namespace and no import path.
		{`Runtime\Go`, "function", `Runtime\Go\len`, ""},
	} {
		if got := g.url(tc.area, tc.kind, tc.name); got != tc.want {
			t.Errorf("url(%q, %q, %q) = %q, want %q", tc.area, tc.kind, tc.name, got, tc.want)
		}
	}
}

// TestSplitClassName covers the area and the name a class name splits into.
func TestSplitClassName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, area, short string
	}{
		{`Runtime\Go\Net\HTTP\Mux`, `Runtime\Go\Net\HTTP`, "Mux"},
		{"stdClass", "", "stdClass"},
	} {
		area, short := splitClassName(tc.name)
		if area != tc.area || short != tc.short {
			t.Errorf("splitClassName(%q) = (%q, %q), want (%q, %q)", tc.name, area, short, tc.area, tc.short)
		}
	}
}

// TestNamed covers the cell: a linked name, and a name the package does not
// hold rendered as plain code.
func TestNamed(t *testing.T) {
	t.Parallel()
	g := newGenerator()
	if got, want := g.named(`Runtime\Go\Errors`, "function", "New"), "[`New`](https://pkg.go.dev/errors#New)"; got != want {
		t.Errorf("named = %q, want %q", got, want)
	}
	if got, want := g.named(`Runtime\Go`, "function", "len"), "`len`"; got != want {
		t.Errorf("named = %q, want %q", got, want)
	}
}
