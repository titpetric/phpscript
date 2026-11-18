package runner

import (
	"slices"
	"testing"
)

// TestTrack covers the report of an install: a name it defines, a name it
// registers on top, a constant, a class, a function apart.
func TestTrack(t *testing.T) {
	t.Parallel()
	rt := New(nil, Options{})
	rt.RegisterFunc("kept", func() int { return 1 })
	rt.RegisterFunc("swapped", func() int { return 1 })
	d := rt.Track(func(rt *Runtime) {
		rt.RegisterFunc("added", func() int { return 2 })
		rt.RegisterFunc("swapped", func() int { return 2 })
		rt.RegisterConstructor("Added\\Class", func() int { return 3 })
	})
	if !slices.Equal(d.Funcs, []string{"added"}) {
		t.Errorf("Funcs = %v", d.Funcs)
	}
	if !slices.Equal(d.Replaced, []string{"swapped"}) {
		t.Errorf("Replaced = %v", d.Replaced)
	}
	if !slices.Equal(d.Classes, []string{"Added\\Class"}) {
		t.Errorf("Classes = %v", d.Classes)
	}
}

// TestNamespacesListed covers the opt-in registry listing by spelling.
func TestNamespacesListed(t *testing.T) {
	RegisterNamespace(`\Test\Listed\Area`, func(*Runtime) {})
	if !slices.Contains(Namespaces(), `Test\Listed\Area`) {
		t.Errorf("not listed: %v", Namespaces())
	}
}
