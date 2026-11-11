package runner

import (
	"sync/atomic"
	"testing"
)

// installedFields records the installer of the area TestAreaFields registers.
//
// The registry holds a registration for the life of the process and the
// default profile is every bit. Every later Register in this binary runs that
// installer, from whichever test goroutine got there, which leaves one reader
// of the flag and a writer per parallel test.
var installedFields atomic.Bool

// TestAreaFields covers the descriptor a registration produces: the name, the
// profile and the installer travel together.
func TestAreaFields(t *testing.T) {
	const profile Profile = 1 << 59
	RegisterProfileArea(`Runtime\PS\Test\Fields`, profile, func(*Runtime) { installedFields.Store(true) })

	areas := AreasFor(profile)
	if len(areas) != 1 || areas[0].Name != `Runtime\PS\Test\Fields` || areas[0].Profile != profile {
		t.Fatalf("got %+v", areas)
	}
	areas[0].Install(nil)
	if !installedFields.Load() {
		t.Error("the installer did not run")
	}
}
