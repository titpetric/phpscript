package list

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestAreas covers the area listing: the strings area is there with functions,
// the overlay layers after it, the Go namespace is opt-in, and a namespace
// argument narrows the rows.
func TestAreas(t *testing.T) {
	rows := Areas(runner.Options{})
	byName := map[string]AreaRow{}
	for _, r := range rows {
		byName[r.Area] = r
	}
	if r := byName[`Runtime\PS\Core\Strings`]; r.Functions == 0 || r.Layer != "ps" {
		t.Errorf("strings area = %+v", r)
	}
	if r := byName[`Runtime\Go`]; r.Layer != "opt-in" || r.Functions == 0 {
		t.Errorf("go area = %+v", r)
	}
	for _, r := range Areas(runner.Options{}, `Runtime\PX`) {
		if r.Layer != "px" {
			t.Errorf("%s is not an overlay area", r.Area)
		}
	}
	if got := AreasMarkdown(rows); !strings.Contains(got, "| Area") || !strings.Contains(got, `Runtime\PS\Core\Strings`) {
		t.Errorf("markdown misses the header or an area:\n%.200s", got)
	}

	for _, r := range Areas(runner.Options{Runtime: runner.RuntimeGo}) {
		if !strings.HasPrefix(r.Area, `Runtime\Go`) {
			t.Errorf("the go listing holds %s", r.Area)
		}
	}
}

// TestStdlibSymbolsCarryAnArea covers the area column of --bindings.
func TestStdlibSymbolsCarryAnArea(t *testing.T) {
	for _, s := range Stdlib(runner.Options{}) {
		if s.Kind == "function" && s.Name == "strlen" && s.Area != `Runtime\PS\Core\Strings` {
			t.Errorf("strlen area = %q", s.Area)
		}
		if s.Area == "" && s.Kind == "function" {
			t.Errorf("function %s has no area", s.Name)
		}
	}
}
