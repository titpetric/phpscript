package stdlib

import (
	"testing"
)

// TestRegisterFSAreasAreRegistered pins that every area RegisterFS gates on is
// one a package registers. A rename cannot silently skip a reroot.
func TestRegisterFSAreasAreRegistered(t *testing.T) {
	registered := map[string]bool{}
	for _, info := range Areas(All) {
		registered[info.Name] = true
	}
	for _, fs := range fsRoots {
		if !registered[fs.area] {
			t.Errorf("RegisterFS gates on %s, which no package registers", fs.area)
		}
	}
}
