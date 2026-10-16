package phpval

import (
	"sort"
	"testing"
)

// superglobals is the closed set the language defines, in the order Range
// visits them.
var superglobals = []string{"_COOKIE", "_ENV", "_FILES", "_GET", "_POST", "_REQUEST", "_SERVER", "_SESSION"}

// TestAutoGlobals_Set covers which names claim a fixed field. The set is
// closed by the language, so a name outside it reports false and the caller
// stores the variable the way it stores every other one.
func TestAutoGlobals_Set(t *testing.T) {
	t.Parallel()
	for _, name := range superglobals {
		var globals AutoGlobals
		if !globals.Set(name, "value") {
			t.Errorf("Set(%q) = false, want the superglobal store", name)
		}
	}

	for _, name := range []string{
		"", "_", "_G", "_GE", "x", "GET", "_get", "_GETX", "_SERVERS", "name", "_1234",
	} {
		var globals AutoGlobals
		if globals.Set(name, "value") {
			t.Errorf("Set(%q) = true, want no superglobal of that name", name)
		}
	}
}

// TestAutoGlobals_Lookup covers the two false answers a read has. A name
// outside the set is not a superglobal, and one inside it that nothing bound
// is unset; both let the caller fall through to globals and constants.
func TestAutoGlobals_Lookup(t *testing.T) {
	t.Parallel()
	var globals AutoGlobals

	for _, name := range superglobals {
		if value, ok := globals.Lookup(name); ok || value != nil {
			t.Errorf("Lookup(%q) on a fresh value = %#v, %t, want nil, false", name, value, ok)
		}
	}

	globals.Set("_GET", map[string]any{"page": "2"})
	value, ok := globals.Lookup("_GET")
	if !ok {
		t.Fatal("Lookup(_GET) = false after Set")
	}
	query, isMap := value.(map[string]any)
	if !isMap || query["page"] != "2" {
		t.Errorf("Lookup(_GET) = %#v, want the stored map", value)
	}

	// A bound superglobal does not bind its neighbours.
	if _, ok := globals.Lookup("_POST"); ok {
		t.Error("Lookup(_POST) = true, want only _GET bound")
	}
	if _, ok := globals.Lookup("page"); ok {
		t.Error("Lookup(page) = true, want false for a name outside the set")
	}
}

// TestAutoGlobals_Range covers the visit order and the skip. Every bound field
// is visited once in the fixed order the fields are declared in, and an unset
// one is passed over, so a request that bound three superglobals registers
// three.
func TestAutoGlobals_Range(t *testing.T) {
	t.Parallel()
	var globals AutoGlobals

	var visited []string
	globals.Range(func(name string, _ any) { visited = append(visited, name) })
	if len(visited) != 0 {
		t.Errorf("Range over a fresh value visited %v, want nothing", visited)
	}

	globals.Set("_SERVER", "server")
	globals.Set("_COOKIE", "cookie")
	globals.Set("_POST", "post")

	seen := map[string]any{}
	visited = nil
	globals.Range(func(name string, value any) {
		visited = append(visited, name)
		seen[name] = value
	})

	want := []string{"_COOKIE", "_POST", "_SERVER"}
	if len(visited) != len(want) {
		t.Fatalf("Range visited %v, want %v", visited, want)
	}
	for i, name := range want {
		if visited[i] != name {
			t.Errorf("Range visited %v, want %v", visited, want)
			break
		}
	}
	if seen["_SERVER"] != "server" || seen["_COOKIE"] != "cookie" || seen["_POST"] != "post" {
		t.Errorf("Range reported %#v, want each name with the value it was set to", seen)
	}
}

// TestAutoGlobals_RangeVisitsEveryField covers the full set: a request that
// bound all eight visits all eight, in the declared field order. That order is
// alphabetical, so the expected list is the sorted one.
func TestAutoGlobals_RangeVisitsEveryField(t *testing.T) {
	t.Parallel()
	var globals AutoGlobals
	for _, name := range superglobals {
		globals.Set(name, name)
	}

	var visited []string
	globals.Range(func(name string, value any) {
		if value != name {
			t.Errorf("Range reported %q for %q", value, name)
		}
		visited = append(visited, name)
	})

	sorted := append([]string(nil), superglobals...)
	sort.Strings(sorted)
	for i, name := range sorted {
		if i >= len(visited) || visited[i] != name {
			t.Fatalf("Range visited %v, want %v", visited, sorted)
		}
	}
	if len(visited) != len(sorted) {
		t.Fatalf("Range visited %v, want %v", visited, sorted)
	}
}

// TestAutoGlobals_Reset covers the per-request clear. Every field drops to
// nil, so the next request's Register rebinds from nothing and a value from
// the last request cannot be read.
func TestAutoGlobals_Reset(t *testing.T) {
	t.Parallel()
	var globals AutoGlobals
	for _, name := range superglobals {
		globals.Set(name, name)
	}

	globals.Reset()

	for _, name := range superglobals {
		if value, ok := globals.Lookup(name); ok {
			t.Errorf("Lookup(%q) after Reset = %#v, true, want unset", name, value)
		}
	}
	var visited int
	globals.Range(func(string, any) { visited++ })
	if visited != 0 {
		t.Errorf("Range after Reset visited %d names, want none", visited)
	}

	// The value is usable again: Reset writes a zero struct and never a nil
	// receiver.
	if !globals.Set("_GET", "again") {
		t.Error("Set(_GET) after Reset = false")
	}
	if value, ok := globals.Lookup("_GET"); !ok || value != "again" {
		t.Errorf("Lookup(_GET) after Reset and Set = %#v, %t", value, ok)
	}
}

// TestAutoGlobals_Claims covers the question a lazy binder asks: whether a
// name belongs to the closed superglobal set at all, bound or not, which
// Lookup cannot answer because an unset superglobal and a plain variable both
// read false there.
func TestAutoGlobals_Claims(t *testing.T) {
	var globals AutoGlobals
	for _, name := range []string{"_GET", "_POST", "_COOKIE", "_SERVER", "_ENV", "_REQUEST", "_FILES", "_SESSION"} {
		if !globals.Claims(name) {
			t.Errorf("Claims(%q) = false, want the whole closed set claimed", name)
		}
	}
	for _, name := range []string{"", "_", "_G", "x", "argv", "_get", "_GETX"} {
		if globals.Claims(name) {
			t.Errorf("Claims(%q) = true, want no claim outside the set", name)
		}
	}
}
