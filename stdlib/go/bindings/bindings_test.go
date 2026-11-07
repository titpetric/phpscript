package bindings_test

import (
	"net/http"
	"testing"

	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// TestFor covers what opens a set: the path maps to the area a script reaches
// the package through, and a path outside the linked subset is a defect the
// binding package hears about at once.
func TestFor(t *testing.T) {
	t.Parallel()
	if area := bindings.For("net/http").Area(); area != `Runtime\Go\Net\HTTP` {
		t.Errorf("net/http opens the area %s", area)
	}
	for _, path := range []string{"net/rpc", "github.com/titpetric/gozero", "Net/HTTP"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s opened a set", path)
				}
			}()
			bindings.For(path)
		}()
	}
}

// TestEntry covers the triple a binding writes: the path of the set, the name
// go spells, the value behind it.
func TestEntry(t *testing.T) {
	t.Parallel()
	entries := bindings.For("net/http").Bind("NewRequest", http.NewRequest).Entries()
	if len(entries) != 1 {
		t.Fatalf("the set holds %d entries, want 1", len(entries))
	}
	want := bindings.Entry{Package: "net/http", Name: "NewRequest", Value: http.NewRequest}
	if entries[0].Package != want.Package || entries[0].Name != want.Name {
		t.Errorf("the entry is %v, want %v", entries[0], want)
	}
	if entries[0].Value == nil {
		t.Error("the entry carries no value")
	}
}
