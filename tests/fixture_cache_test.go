package tests

import (
	"testing"
)

// TestShareSources covers the layer precompiling one directory compiles from.
// Every body in the directory sits in one filesystem, so one compile covers
// the whole area and no fixture in it compiles the tree again.
func TestShareSources(t *testing.T) {
	first := &Fixture{Name: "first", Path: "area/first.phpt", PHP: "<?php echo 1;"}
	second := &Fixture{Name: "second", Path: "area/second.phpt", PHP: "<?php echo 2;"}

	ShareSources([]*Fixture{first, second})

	for _, fx := range []*Fixture{first, second} {
		if got := len(fx.sources); got != 2 {
			t.Errorf("%s carries %d bodies, want 2", fx.Name, got)
		}
		if _, ok := fx.sources[first.sourceName()]; !ok {
			t.Errorf("%s does not carry %s", fx.Name, first.sourceName())
		}
		if _, ok := fx.sources[second.sourceName()]; !ok {
			t.Errorf("%s does not carry %s", fx.Name, second.sourceName())
		}
	}

	// A fixture naming its own root keys a cache of its own, so the bodies of
	// one directory do not reach the tree a second directory compiled.
	apart := &Fixture{Name: "apart", Path: "other/apart.phpt", PHP: "<?php echo 3;"}
	ShareSources([]*Fixture{first, apart})
	if len(apart.sources) != 1 {
		t.Errorf("apart carries %d bodies, want 1", len(apart.sources))
	}
}
