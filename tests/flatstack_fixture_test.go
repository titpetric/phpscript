package tests

import (
	"io/fs"
	"strings"
	"testing"
)

// TestFlatstackFixtures runs every fixture through the flat bytecode runtime,
// which falls back to the compatibility interpreter for syntax it does not
// compile yet.
func TestFlatstackFixtures(t *testing.T) {
	entries, err := fs.ReadDir(fixturesFS, "fixtures")
	if err != nil {
		t.Fatal(err)
	}
	selected := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".phpt") {
			continue
		}
		data, err := fixturesFS.ReadFile("fixtures/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		fx, err := ParseFixture(data, "fixtures/"+entry.Name())
		if err != nil {
			t.Fatal(err)
		}
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
	if selected == 0 {
		t.Fatal("no flatstack fixtures selected")
	}
}
