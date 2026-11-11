package stdlib

import (
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestOwned covers the first area to define a name owning it, one table per
// kind. A name two areas define belongs to the one that installs first.
func TestOwned(t *testing.T) {
	owned := Owners([]AreaInfo{
		{Name: `Runtime\PS\A`, Delta: runner.Delta{Funcs: []string{"f"}, Classes: []string{"C"}, Constants: []string{"K"}}},
		{Name: `Runtime\PX\B`, Delta: runner.Delta{Funcs: []string{"f", "g"}}},
	})
	if owned.Funcs["f"] != `Runtime\PS\A` || owned.Funcs["g"] != `Runtime\PX\B` {
		t.Errorf("functions owned wrongly: %v", owned.Funcs)
	}
	if owned.Classes["C"] != `Runtime\PS\A` || owned.Constants["K"] != `Runtime\PS\A` {
		t.Errorf("a class or constant owned wrongly: %v %v", owned.Classes, owned.Constants)
	}
}
