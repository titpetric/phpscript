package apidoc

import (
	"io"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestRenderGroupsByArea covers the page a run with owners writes: a heading per
// namespace group, a subheading per area, the class naming its area, and an
// unowned function filed under other.
func TestRenderGroupsByArea(t *testing.T) {
	t.Parallel()
	funcs := []Func{
		{Name: "strlen", Returns: "int"},
		{Name: "strcmp", Returns: "int"},
		{Name: "array_map", Returns: "array"},
		{Name: "orphan", Returns: "void"},
	}
	classes := []Class{{Name: "Time"}}
	owners := map[string]string{
		"strlen":    `Runtime\PS\Core\Strings`,
		"strcmp":    `Runtime\PS\Core\Strings\Compare`,
		"array_map": `Runtime\PS\Core\Arrays`,
	}
	annotateNamespaces(funcs, classes, Owners{Funcs: owners, Classes: map[string]string{"Time": `Runtime\PS\Time`}})
	page := render(funcs, classes)

	for _, want := range []string{
		"### Runtime\\PS\\Core\n",
		"#### Runtime\\PS\\Core\\Arrays\n",
		"#### Runtime\\PS\\Core\\Strings\n",
		"#### Runtime\\PS\\Core\\Strings\\Compare\n",
		"### other\n",
		"Area `Runtime\\PS\\Time`.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "\n### Runtime\\PS\\Core\\Strings\n") {
		t.Error("a deep area got a group heading")
	}
}

// TestAreaGroup covers the group an area files under.
func TestAreaGroup(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                                `other`,
		`Runtime\PS\Time`:                 `Runtime\PS\Time`,
		`Runtime\PS\Core\Strings\Compare`: `Runtime\PS\Core`,
		`Runtime\PX\Dates`:                `Runtime\PX\Dates`,
	}
	for in, want := range cases {
		if got := areaGroup(in); got != want {
			t.Errorf("areaGroup(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestGenerateAreas covers the entry point with an owners map: the page groups by
// namespace where Generate groups by Go package.
func TestGenerateAreas(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	rt.RegisterFunc("probe_area_fn", func() int { return 1 })
	page, err := GenerateAreas(rt, "../..", Owners{Funcs: map[string]string{"probe_area_fn": `Runtime\PS\Test\Probe`}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "### Runtime\\PS\\Test\\Probe\n") || !strings.Contains(page, "function probe_area_fn(): int") {
		t.Errorf("page lacks the area heading or the function")
	}
	plain, err := Generate(rt, "../..")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "Runtime\\PS\\Test\\Probe") {
		t.Error("Generate named an area")
	}
}
