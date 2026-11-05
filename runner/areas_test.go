package runner

import (
	"slices"
	"strings"
	"testing"
)

// TestRegisterArea covers the area registry: a named area reaches the mask it
// registered under, a nil installer is dropped, and an empty name is filed under
// the unnamed area.
func TestRegisterArea(t *testing.T) {
	const area Profile = 1 << 61
	RegisterProfileArea(`Runtime\PS\Test\Named`, area, func(*Runtime) {})
	RegisterProfileArea("", area, func(*Runtime) {})
	RegisterProfileArea(`Runtime\PS\Test\Dropped`, area, nil)

	var names []string
	for _, a := range AreasFor(area) {
		names = append(names, a.Name)
	}
	if want := []string{`Runtime\PS\Test\Named`, NamespaceUnnamed}; !slices.Equal(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	if got := AreasFor(ProfileSecure &^ area); slices.ContainsFunc(got, func(a Area) bool { return a.Profile == area }) {
		t.Error("the test area reached the secure profile")
	}
}

// TestAreasInstallOverlayLast covers the order: a Runtime\PX area installs after
// every Runtime\PS area that registered after it.
func TestAreasInstallOverlayLast(t *testing.T) {
	const area Profile = 1 << 60
	RegisterProfileArea(`Runtime\PX\Test\Over`, area, func(*Runtime) {})
	RegisterProfileArea(`Runtime\PS\Test\Under`, area, func(*Runtime) {})

	got := AreasFor(area)
	if len(got) != 2 || got[0].Name != `Runtime\PS\Test\Under` || got[1].Name != `Runtime\PX\Test\Over` {
		t.Fatalf("overlay not last: %v", got)
	}
}

// TestSelectAreas covers the default runtime's selection: a namespace keeps the
// areas below it, an empty list keeps all, and the match ignores case and a
// leading separator.
func TestSelectAreas(t *testing.T) {
	t.Parallel()
	all := []Area{{Name: `Runtime\PS\Core\Strings`}, {Name: `Runtime\PS\Http`}, {Name: `Runtime\PX\Dates`}, {Name: `Runtime\PSX\Other`}}
	cases := []struct {
		namespaces []string
		want       int
	}{
		{nil, 4},
		{[]string{`Runtime\PS`}, 2},
		{[]string{`\runtime\px`}, 1},
		{[]string{`Runtime\PS\Core`, `Runtime\PX`}, 2},
		{[]string{`Runtime\Go`}, 0},
	}
	for _, tc := range cases {
		if got := len(SelectAreas(all, tc.namespaces)); got != tc.want {
			t.Errorf("%v kept %d areas, want %d", tc.namespaces, got, tc.want)
		}
	}
}

// TestRuntime_Areas covers the accessor: a runtime reports the namespaces its options named.
func TestRuntime_Areas(t *testing.T) {
	t.Parallel()
	if got := New(nil, Options{Areas: []string{`Runtime\PS`}}).Areas(); len(got) != 1 || got[0] != `Runtime\PS` {
		t.Errorf("got %v", got)
	}
	if got := New(nil, Options{}).Areas(); len(got) != 0 {
		t.Errorf("a default runtime names areas: %v", got)
	}
}

// TestCheckAreas covers the validation: a name that matches an area or sits
// above one passes, and a misspelling is refused with the name in the message.
func TestCheckAreas(t *testing.T) {
	RegisterArea(`Runtime\PS\Test\Checked`, func(*Runtime) {})
	if err := CheckAreas([]string{`Runtime\PS`, `runtime\ps\test\checked`}); err != nil {
		t.Errorf("valid names refused: %v", err)
	}
	err := CheckAreas([]string{`Runtime\PS\Test\Chekced`})
	if err == nil || !strings.Contains(err.Error(), "Chekced") {
		t.Errorf("a misspelling was accepted: %v", err)
	}
	if err := CheckAreas([]string{NamespacePS, NamespacePX, NamespaceGo}); err != nil {
		t.Errorf("a root was refused: %v", err)
	}
	if CheckAreas(nil) != nil {
		t.Error("an empty selection was refused")
	}
}

// TestSelectNone covers Runtime\None: it selects no area, whatever else the
// list names, and CheckAreas accepts it as a root.
func TestSelectNone(t *testing.T) {
	areas := []Area{{Name: `Runtime\PS\Core\Strings`}, {Name: `Runtime\PS\Files`}}
	for _, list := range [][]string{{NamespaceNone}, {NamespacePS, `runtime\none`}} {
		if got := SelectAreas(areas, list); len(got) != 0 {
			t.Errorf("%v selected %d areas, want none", list, len(got))
		}
	}
	if got := SelectAreas(areas, nil); len(got) != 2 {
		t.Errorf("an empty list selected %d areas, want all", len(got))
	}
	if err := CheckAreas([]string{NamespaceNone}); err != nil {
		t.Errorf("Runtime\\None was refused: %v", err)
	}
}
