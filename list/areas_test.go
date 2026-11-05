package list

import (
	"strings"
	"testing"
)

// TestAreas covers the area listing: the strings area is there with functions,
// the overlay layers after it, the Go namespace is opt-in, and a namespace
// argument narrows the rows.
func TestAreas(t *testing.T) {
	rows := Areas()
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
	for _, r := range Areas(`Runtime\PX`) {
		if r.Layer != "px" {
			t.Errorf("%s is not an overlay area", r.Area)
		}
	}
	if got := AreasMarkdown(rows); !strings.Contains(got, "| Area") || !strings.Contains(got, `Runtime\PS\Core\Strings`) {
		t.Errorf("markdown misses the header or an area:\n%.200s", got)
	}
}

// TestStdlibSymbolsCarryAnArea covers the area column of --bindings.
func TestStdlibSymbolsCarryAnArea(t *testing.T) {
	for _, s := range Stdlib() {
		if s.Kind == "function" && s.Name == "strlen" && s.Area != `Runtime\PS\Core\Strings` {
			t.Errorf("strlen area = %q", s.Area)
		}
		if s.Area == "" && s.Kind == "function" {
			t.Errorf("function %s has no area", s.Name)
		}
	}
}
