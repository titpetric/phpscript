package main

import (
	"testing"
)

// TestVersion covers the defaults a build with no ldflags carries. "unknown" is
// not a placeholder here: the version command prints the commit, the commit
// time and the branch only when each says something else, so a `go build` with
// no VCS information prints the version line alone, with no three lines
// reading unknown.
func TestVersion(t *testing.T) {
	t.Parallel()
	if Version != "dev" {
		t.Errorf("Version = %q, want the development default", Version)
	}
	for name, value := range map[string]string{
		"Commit":     Commit,
		"CommitTime": CommitTime,
		"Branch":     Branch,
	} {
		if value != "unknown" {
			t.Errorf("%s = %q, want the sentinel the version command drops", name, value)
		}
	}
}
