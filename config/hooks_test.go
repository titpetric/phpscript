package config

import (
	"testing"
)

// TestHooks covers the two fields a suite root declares its session SQL in.
// Both are paths resolved against that root, and both are optional: a suite
// that declares neither runs its fixtures with nothing around them.
func TestHooks(t *testing.T) {
	t.Parallel()
	hooks := Hooks{Setup: "setup.php", Teardown: "teardown.php"}
	if hooks.Setup != "setup.php" || hooks.Teardown != "teardown.php" {
		t.Errorf("Hooks lost a field: %+v", hooks)
	}

	if empty := (Hooks{}); empty.Setup != "" || empty.Teardown != "" {
		t.Errorf("the zero Hooks names a file: %+v", empty)
	}

	// Teardown alone is legal: a suite may clean up state a fixture created
	// without declaring any to begin with.
	if only := (Hooks{Teardown: "drop.php"}); only.Setup != "" {
		t.Errorf("declaring Teardown filled Setup: %+v", only)
	}
}
