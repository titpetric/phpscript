package tests

import (
	"io/fs"
	"strings"
	"testing"
)

// TestFlatstackFixtures runs opted-in fixtures through flat bytecode.
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
		if !fx.Flatstack {
			continue
		}
		selected++
		t.Run(fx.Name, func(t *testing.T) {
			res := RunFixture(t.Context(), fx)
			if !res.Passed {
				t.Fatalf("flatstack fixture failed: %s", res.FailureReason)
			}
		})
	}
	if selected == 0 {
		t.Fatal("no flatstack fixtures selected")
	}
}
