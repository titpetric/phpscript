package apidoc

import (
	"encoding"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"
)

// TestCitation covers the four spellings one citation answers in: the
// qualified Go symbol a description cites, the selector source writes, the
// markdown link, and the name an implements clause prints.
func TestCitation(t *testing.T) {
	t.Parallel()
	c := Citation{
		Package: "net/http",
		Name:    "Client",
		Go:      "http.Client",
		URL:     "https://pkg.go.dev/net/http#Client",
	}
	if got, want := c.Symbol(), "net/http.Client"; got != want {
		t.Errorf("Symbol() = %q, want %q", got, want)
	}
	if got, want := c.Short(), "http.Client"; got != want {
		t.Errorf("Short() = %q, want %q", got, want)
	}
	if got, want := c.Link(), "[net/http.Client](https://pkg.go.dev/net/http#Client)"; got != want {
		t.Errorf("Link() = %q, want %q", got, want)
	}
	if got, want := c.Clause(), "http.Client"; got != want {
		t.Errorf("Clause() = %q, want %q", got, want)
	}
}

// TestCitation_Symbol covers the qualified spelling. A builtin has no import
// path to qualify with.
func TestCitation_Symbol(t *testing.T) {
	t.Parallel()
	if got, want := (Citation{Name: "error"}).Symbol(), "error"; got != want {
		t.Errorf("Symbol() = %q, want %q", got, want)
	}
}

// TestCitation_Short covers the selector Go source writes, which is not the
// last element of the import path: encoding/json/v2 declares json.Marshaler,
// and a citation that guessed from the path would read v2.Marshaler.
func TestCitation_Short(t *testing.T) {
	t.Parallel()
	c := cite(reflect.TypeOf((*json.Marshaler)(nil)).Elem())
	if got := c.Short(); got != "json.Marshaler" {
		t.Errorf("Short() = %q, want %q", got, "json.Marshaler")
	}
	if got, want := (Citation{Name: "error"}).Short(), "error"; got != want {
		t.Errorf("Short() with no selector = %q, want %q", got, want)
	}
}

// TestCitation_Link covers the one rule on links: a standard library symbol
// links its reference page and the listing names every other symbol bare,
// because no page exists to point a reader at.
func TestCitation_Link(t *testing.T) {
	t.Parallel()
	module := Citation{Package: "github.com/titpetric/phpscript/runner", Name: "Throwable", Go: "runner.Throwable"}
	if got, want := module.Link(), "github.com/titpetric/phpscript/runner.Throwable"; got != want {
		t.Errorf("Link() = %q, want the bare symbol %q", got, want)
	}
}

// TestCitation_Clause covers the name an implements clause prints: the PHP
// spelling when a script can type one, the Go selector otherwise.
func TestCitation_Clause(t *testing.T) {
	t.Parallel()
	c := Citation{Package: "x", Name: "Throwable", Go: "x.Throwable", PHP: "Throwable"}
	if got, want := c.Clause(), "Throwable"; got != want {
		t.Errorf("Clause() = %q, want %q", got, want)
	}
}

// TestCite covers what reflection recovers about a Go type: the import path,
// the name, the selector, and a reference page for a standard library type and
// for none other.
func TestCite(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ  reflect.Type
		want Citation
	}{
		{
			// A pointer cites the type it points at: the class binds a
			// *http.Client and net/http documents Client.
			typ:  reflect.TypeOf(&http.Client{}),
			want: Citation{Package: "net/http", Name: "Client", Go: "http.Client", URL: "https://pkg.go.dev/net/http#Client"},
		},
		{
			typ:  reflect.TypeOf(time.Duration(0)),
			want: Citation{Package: "time", Name: "Duration", Go: "time.Duration", URL: "https://pkg.go.dev/time#Duration"},
		},
		{
			typ:  reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem(),
			want: Citation{Package: "encoding", Name: "TextMarshaler", Go: "encoding.TextMarshaler", URL: "https://pkg.go.dev/encoding#TextMarshaler"},
		},
		{
			// error is the one interface the language itself declares,
			// and builtin is where its page lives.
			typ:  reflect.TypeOf((*error)(nil)).Elem(),
			want: Citation{Name: "error", Go: "error", URL: "https://pkg.go.dev/builtin#error"},
		},
		{
			typ:  reflect.TypeOf(citedLocal{}),
			want: Citation{Package: "github.com/titpetric/phpscript/internal/apidoc", Name: "citedLocal", Go: "apidoc.citedLocal"},
		},
	}
	for _, c := range cases {
		if got := cite(c.typ); got != c.want {
			t.Errorf("cite(%s) = %+v, want %+v", c.typ, got, c.want)
		}
	}
}

// TestIsStdlib covers the test that separates a standard library import path
// from a module one: the first element of a module path holds a dot, the test
// the go command applies.
func TestIsStdlib(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"net/http":                            true,
		"time":                                true,
		"encoding/json/v2":                    true,
		"github.com/titpetric/phpscript":      false,
		"github.com/titpetric/phpscript/http": false,
		"":                                    false,
	}
	for pkg, want := range cases {
		if got := isStdlib(pkg); got != want {
			t.Errorf("isStdlib(%q) = %v, want %v", pkg, got, want)
		}
	}
}

// citedLocal is a type this module declares. It cites itself and links
// nowhere.
type citedLocal struct{}
