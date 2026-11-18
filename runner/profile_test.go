package runner

import (
	"io"
	"testing"
)

// TestProfile covers the bit layout behind every area name: one bit per area.
// A mask naming two areas holds both bits, overlapping each alone. A second
// area sharing a bit with the original would mount alongside it.
func TestProfile(t *testing.T) {
	t.Parallel()
	if ProfileSecure == ProfileExec {
		t.Fatalf("ProfileSecure and ProfileExec are the same bit, %b", ProfileSecure)
	}
	if ProfileSecure&ProfileExec != 0 {
		t.Errorf("ProfileSecure&ProfileExec = %b, want 0", ProfileSecure&ProfileExec)
	}

	both := ProfileSecure | ProfileExec
	for _, area := range []Profile{ProfileSecure, ProfileExec} {
		if both&area == 0 {
			t.Errorf("(ProfileSecure|ProfileExec)&%b = 0, want the area", area)
		}
		if ^Profile(0)&area == 0 {
			t.Errorf("every area &%b = 0, want the area", area)
		}
	}
}

// TestRuntime_SetProfile covers the mask the runtime records: a reroot of one
// area, stdlib.RegisterFS over pexec, tests the runtime for the area before it
// reroots anything. A runtime nobody mounted answers every area; a host that
// called Register and not Mount is not read as a narrowed one.
func TestRuntime_SetProfile(t *testing.T) {
	rt := New(io.Discard, Options{})
	if rt.Profile()&ProfileExec == 0 {
		t.Errorf("a fresh runtime answers %b for Exec, want the area", rt.Profile())
	}

	rt.SetProfile(ProfileSecure)
	if got := rt.Profile(); got != ProfileSecure {
		t.Errorf("Profile() = %b, want %b", got, ProfileSecure)
	}
	if rt.Profile()&ProfileExec != 0 {
		t.Errorf("Profile()&ProfileExec = %b after narrowing to Secure, want 0", rt.Profile()&ProfileExec)
	}
}
