package runner

import (
	"testing"
)

// profileTestArea is a bit no binding package registers under, so a test
// registering one installer under it does not reach a runtime any other test
// mounts. The registry is process-wide and append-only: an entry cannot be
// taken back out, so the area it is filed under is what keeps it contained.
const profileTestArea Profile = 1 << 62

// TestRegisterProfileBinding covers what an area bit decides. An installer
// filed under one area is returned for a mask naming it and withheld from one
// that does not, so a narrowed mount leaves a name undefined and never refuses
// it at the call.
//
// A nil installer is dropped at registration, because a contributed nil would
// panic inside Mount, after the host had already installed part of the surface.
func TestRegisterProfileBinding(t *testing.T) {
	before := len(BindingsFor(profileTestArea))

	RegisterProfileBinding(profileTestArea, func(*Runtime) {})
	if got := len(BindingsFor(profileTestArea)); got != before+1 {
		t.Fatalf("BindingsFor(test area) returned %d installers, want %d", got, before+1)
	}
	if got := len(BindingsFor(ProfileSecure &^ profileTestArea)); got != len(BindingsFor(ProfileSecure)) {
		t.Errorf("the test area reached ProfileSecure: %d installers against %d", got, len(BindingsFor(ProfileSecure)))
	}

	RegisterProfileBinding(profileTestArea, nil)
	if got := len(BindingsFor(profileTestArea)); got != before+1 {
		t.Errorf("a nil installer was registered: %d installers, want %d", got, before+1)
	}
}

// TestBindingsFor covers the two masks a host reaches the registry through.
// Bindings is every area, the surface stdlib.Register mounts, so it is never
// shorter than any single area's share of it.
func TestBindingsFor(t *testing.T) {
	RegisterProfileBinding(profileTestArea, func(*Runtime) {})

	all := len(Bindings())
	area := len(BindingsFor(profileTestArea))
	if area == 0 {
		t.Fatal("BindingsFor(test area) returned nothing, so nothing registered under it")
	}
	if all < area {
		t.Errorf("Bindings() returned %d installers, fewer than the %d under one area", all, area)
	}
	if got := len(BindingsFor(0)); got != 0 {
		t.Errorf("BindingsFor(0) returned %d installers, want none", got)
	}
}
