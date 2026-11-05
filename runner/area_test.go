package runner

import (
	"testing"
)

// TestAreaFields covers the descriptor a registration produces: the name, the
// profile and the installer travel together.
func TestAreaFields(t *testing.T) {
	const profile Profile = 1 << 59
	installed := false
	RegisterProfileArea(`Runtime\PS\Test\Fields`, profile, func(*Runtime) { installed = true })

	areas := AreasFor(profile)
	if len(areas) != 1 || areas[0].Name != `Runtime\PS\Test\Fields` || areas[0].Profile != profile {
		t.Fatalf("got %+v", areas)
	}
	areas[0].Install(nil)
	if !installed {
		t.Error("the installer did not run")
	}
}
