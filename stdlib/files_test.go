package stdlib

import (
	"testing"
)

// TestRegisterFSAreasAreRegistered pins every area RegisterFS gates on to one
// a package registers. A rename that skipped a reroot fails here.
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
