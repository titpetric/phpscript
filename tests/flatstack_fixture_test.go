package tests

import (
	"testing"
)

// TestFlatstackFixtures runs every fixture through the flat bytecode runtime,
// which falls back to the compatibility interpreter for syntax it does not
// compile yet.
func TestFlatstackFixtures(t *testing.T) {
	areas, err := embeddedFixtures()
	if err != nil {
		t.Fatal(err)
	}
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
