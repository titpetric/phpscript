package apidoc

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"
)

// boundClient is the shape a binding of this module takes: it holds the
// standard library type it exposes, next to the machinery it needs to.
type boundClient struct {
	client  *http.Client
	timeout time.Duration
}

// boundEncoder wraps a type whose name it extends, the JSONEncoder over
// json.Encoder shape, and holds the writer that encoder writes to.
type boundEncoder struct {
	writer  io.Writer
	encoder *json.Encoder
}

// boundStore holds machinery and exposes nothing of the standard library, the
// SharedMemory over sync.Mutex shape.
type boundStore struct {
	mu     sync.Mutex
	values map[string]string
}

// boundEmbedded reaches its standard library type by embedding it.
type boundEmbedded struct {
	*http.ServeMux
}

// TestReflectBinds covers the rule the class description cites by: a bound type
// the standard library declares cites itself, one this module declares cites
// the standard library type it wraps, and one that wraps none cites itself.
//
// The name test separates the wrapped value from the machinery beside it.
// Without it an encoder cites the io.Writer it holds, the first standard
// library field it declares and the one it encodes through.
func TestReflectBinds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeOf(&http.Client{}), "net/http.Client"},
		{reflect.TypeOf(time.Time{}), "time.Time"},
		{reflect.TypeOf(&boundClient{}), "net/http.Client"},
		{reflect.TypeOf(&boundEncoder{}), "encoding/json.Encoder"},
		{reflect.TypeOf(&boundEmbedded{}), "net/http.ServeMux"},
		{
			// A store that wraps no standard library type cites
			// itself: the sync.Mutex guards the map and a script
			// constructs neither.
			reflect.TypeOf(&boundStore{}),
			"github.com/titpetric/phpscript/internal/apidoc.boundStore",
		},
	}
	for _, c := range cases {
		if got := reflectBinds(c.typ).Symbol(); got != c.want {
			t.Errorf("reflectBinds(%s) cites %q, want %q", c.typ, got, c.want)
		}
	}
}

// TestReflectBindsLinksStdlibOnly pins which citation holds a link. A
// pkg.go.dev page exists for a standard library symbol and for nothing in this
// module, and a link to a page that does not exist is worse than no link.
func TestReflectBindsLinksStdlibOnly(t *testing.T) {
	t.Parallel()
	if got, want := reflectBinds(reflect.TypeOf(&boundClient{})).URL, "https://pkg.go.dev/net/http#Client"; got != want {
		t.Errorf("wrapped standard library citation URL = %q, want %q", got, want)
	}
	if got := reflectBinds(reflect.TypeOf(&boundStore{})).URL; got != "" {
		t.Errorf("module citation URL = %q, want none", got)
	}
}

// TestRelatedNames covers the name relation a wrapper shares with the type it
// wraps: one name ends with the other, case aside.
func TestRelatedNames(t *testing.T) {
	t.Parallel()
	cases := []struct {
		owner, inner string
		want         bool
	}{
		{"Client", "Client", true},
		{"Mux", "ServeMux", true},
		{"JSONEncoder", "Encoder", true},
		{"SharedMemory", "Mutex", false},
		{"Server", "Handler", false},
	}
	for _, c := range cases {
		if got := relatedNames(c.owner, c.inner); got != c.want {
			t.Errorf("relatedNames(%q, %q) = %v, want %v", c.owner, c.inner, got, c.want)
		}
	}
}

// TestGoTypeKey covers the key the doc index holds a bound type's source text
// under. A pointer and the type it points at answer the same key, and an
// unnamed type answers the empty string.
func TestGoTypeKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeOf(&http.Client{}), "net/http.Client"},
		{reflect.TypeOf(http.Client{}), "net/http.Client"},
		{reflect.TypeOf(time.Duration(0)), "time.Duration"},
		{reflect.TypeOf(struct{ A int }{}), ""},
	}
	for _, c := range cases {
		if got := goTypeKey(c.typ); got != c.want {
			t.Errorf("goTypeKey(%s) = %q, want %q", c.typ, got, c.want)
		}
	}
}
