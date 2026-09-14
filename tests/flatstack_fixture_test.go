package tests

import (
	"io"
	"testing"
)

// TestFlatstackFixtures runs every fixture through the flat bytecode runtime.
// A fixture the compiler rejects is skipped, not delegated: a fallback run
// would test the compatibility interpreter a second time under this name.
func TestFlatstackFixtures(t *testing.T) {
	areas, err := embeddedFixtures()
	if err != nil {
		t.Fatal(err)
	}
	teardown, err := setupAreas(t.Context(), areas, io.Discard)
	if err != nil {
		t.Fatalf("suite setup: %v", err)
	}
	t.Cleanup(func() {
		if err := teardown(); err != nil {
			t.Errorf("suite teardown: %v", err)
		}
	})
	selected := 0
	for _, area := range areas {
		t.Run(area.Name, func(t *testing.T) {
			for _, fx := range area.Fixtures {
				if !fx.Runs(RunnerFlatstack) {
					continue
				}
				selected++
				t.Run(fx.Name, func(t *testing.T) {
					res := RunFixtureOn(t.Context(), fx, RunnerFlatstack)
					if res.Skipped {
						t.Skip(res.FailureReason)
					}
					if !res.Passed {
						t.Fatalf("flatstack fixture failed: %s", res.FailureReason)
					}
				})
			}
		})
	}
	if selected == 0 {
		t.Fatal("no flatstack fixtures selected")
	}
}
