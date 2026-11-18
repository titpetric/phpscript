package runner

import (
	"testing"
)

// profileTestArea is a bit no binding package registers. A test
// registering one installer under that area does not reach a runtime any other
// test mounts. The registry is process-wide and append-only: no call removes an
// entry. The area keeps it contained.
const profileTestArea Profile = 1 << 62

// TestRegisterProfileBinding covers the decision an area bit makes. BindingsFor
// answers an installer for a mask naming its area. It withholds the installer
// from a mask that names another. A narrowed mount leaves a name undefined. It
// does not refuse the call.
//
// RegisterProfileBinding drops a nil installer: a contributed nil would panic
// inside Mount, after the host had already installed part of the surface.
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

// TestRegisterBinding covers the area a binding lands in when it names none,
// which is ProfileSecure. A binding package's init() calls this, and the default
// decides the surface a host gets when it mounts the sandbox. The installer
// changes no state: the registry is append-only, and whatever this test
// registers runs on every Runtime the test binary sets up.
func TestRegisterBinding(t *testing.T) {
	secure, outside := len(BindingsFor(ProfileSecure)), len(BindingsFor(^ProfileSecure))

	RegisterBinding(func(*Runtime) {})
	if got := len(BindingsFor(ProfileSecure)); got != secure+1 {
		t.Fatalf("BindingsFor(ProfileSecure) returned %d installers, want %d", got, secure+1)
	}
	// Secure is one area and no mask over every one and BindingsFor withholds
	// the installer from every profile that does not name it.
	if got := len(BindingsFor(^ProfileSecure)); got != outside {
		t.Errorf("the installer reached a profile without ProfileSecure: %d installers, want %d", got, outside)
	}
}

// TestBindingsFor covers the two masks a host reaches the registry through.
// Bindings covers all areas, the surface stdlib.Register mounts, and holds at
// least as much as any single area's share.
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
