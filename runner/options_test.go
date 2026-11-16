package runner

import (
	"testing"
)

// TestOptions_Check covers the runtime names and the areas refusal.
func TestOptions_Check(t *testing.T) {
	for _, mode := range []string{"", RuntimePHPScript, "PHPScript", "go", " GO "} {
		if err := (Options{Runtime: mode}).Check(); err != nil {
			t.Errorf("runtime %q: %v", mode, err)
		}
	}
	if err := (Options{Runtime: "php"}).Check(); err == nil {
		t.Error("an unknown runtime passed the check")
	}
	if err := (Options{Runtime: RuntimeGo, Areas: []string{NamespacePS}}).Check(); err == nil {
		t.Error("runtime go beside areas passed the check")
	}
}

// TestOptions_GoRuntime covers the folded spelling and the default.
func TestOptions_GoRuntime(t *testing.T) {
	if !(Options{Runtime: "Go"}).GoRuntime() {
		t.Error("Go did not select the go runtime")
	}
	if (Options{}).GoRuntime() {
		t.Error("the default selected the go runtime")
	}
	if (Options{Runtime: "php"}).GoRuntime() {
		t.Error("an unknown runtime selected the go runtime")
	}
}

// TestOptions_SelectedAreas covers the namespace each mode installs.
func TestOptions_SelectedAreas(t *testing.T) {
	areas := []string{NamespacePS}
	if got := (Options{Areas: areas}).SelectedAreas(); len(got) != 1 || got[0] != NamespacePS {
		t.Errorf("phpscript selected %v", got)
	}
	if got := (Options{Runtime: RuntimeGo}).SelectedAreas(); len(got) != 1 || got[0] != NamespaceGo {
		t.Errorf("go selected %v", got)
	}
}
