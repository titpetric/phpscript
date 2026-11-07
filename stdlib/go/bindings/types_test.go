package bindings_test

import (
	"net/http"
	"testing"

	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// TestSet_Types covers the walk over the bound signatures: the package's own
// types come back under their own names, a type another package declares does
// not, and a set that binds no function reaches none.
func TestSet_Types(t *testing.T) {
	t.Parallel()
	types := bindings.For("net/http").
		Bind("NewRequest", http.NewRequest).
		Bind("Get", http.Get).
		Types()
	// NewRequest returns *http.Request, whose fields reach the rest.
	for _, name := range []string{"Request", "Response", "Header", "Cookie"} {
		if _, ok := types[name]; !ok {
			t.Errorf("%s is not among the types the signatures reach: %v", name, names(types))
		}
	}
	// url.URL is a field of http.Request, and url declares it. It reaches a
	// script through the field and under no name of this area.
	if _, ok := types["URL"]; ok {
		t.Error("a type of another package took a name of this area")
	}
	if got := bindings.For("net/http").Bind("StatusOK", http.StatusOK).Types(); len(got) != 0 {
		t.Errorf("a constant reached the types %v", names(got))
	}
}

// TestSet_Type covers the seed a binding files for a type no signature
// reaches: the walk starts from it, and what it reaches comes with it.
func TestSet_Type(t *testing.T) {
	t.Parallel()
	types := bindings.For("net/http").Type(http.Client{}).Types()
	if _, ok := types["Client"]; !ok {
		t.Errorf("the seeded type is not among %v", names(types))
	}
	// The client's fields reach the rest of the package.
	if _, ok := types["CookieJar"]; !ok {
		t.Errorf("the seed reached nothing: %v", names(types))
	}
	if got := bindings.For("net/http").Type(nil).Types(); len(got) != 0 {
		t.Errorf("a nil sample seeded %v", names(got))
	}
}

// names lists the keys of a type map, for a failure that reads.
func names[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	return out
}
