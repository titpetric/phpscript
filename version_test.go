package main

import (
	"testing"
)

// TestVersion covers the defaults a build lacking ldflags keeps. "unknown" is
// not a placeholder here: the version command prints the commit, the commit
// time, plus the branch only when each says something else; a `go build`
// lacking VCS information prints the version line alone. Three lines reading
// unknown never appear.
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
