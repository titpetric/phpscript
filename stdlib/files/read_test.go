package files_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// sourceTree is the read-only application an embedded or in-memory host ships.
// It is deliberately not on disk, so a read that answers from it proves the
// fs.FS was consulted rather than the host filesystem underneath.
func sourceTree() fstest.MapFS {
	return fstest.MapFS{
		"one.txt":          {Data: []byte("one"), ModTime: sourceModTime},
		"two.txt":          {Data: []byte("two")},
		"notes.md":         {Data: []byte("notes")},
		"drafts/three.txt": {Data: []byte("three")},
	}
}

// sourceModTime is stamped on one entry so filemtime has something to report
// that only the source filesystem knows. A zero MapFS mod time is the year 1,
// which is a valid answer and so cannot be told apart from a miss.
var sourceModTime = time.Unix(1756600000, 0)

// TestGlobListsTheSourceFilesystem pins the ordinary case: patterns resolve
// inside the bound fs.FS and answer in the spelling they were written in.
func TestGlobListsTheSourceFilesystem(t *testing.T) {
	opts := runner.Options{RootFS: sourceTree()}
	for _, test := range []struct {
		pattern string
		want    string
	}{
		{pattern: "*.txt", want: "one.txt,two.txt"},
		{pattern: "*", want: "drafts,notes.md,one.txt,two.txt"},
		{pattern: "*/*.txt", want: "drafts/three.txt"},
		{pattern: "o?e.txt", want: "one.txt"},
		{pattern: "*.json", want: ""},
	} {
		t.Run(test.pattern, func(t *testing.T) {
			got := runFSOptions(t, t.TempDir(), nil, opts, `<?php echo implode(",", glob("`+test.pattern+`"));`)
			if got != test.want {
				t.Errorf("glob(%q) = %q, want %q", test.pattern, got, test.want)
			}
		})
	}
}

// TestGlobIsJailedToTheSourceFilesystem pins where "/" points. It is the source
// filesystem's root, not the host's, so an absolute pattern names something
// inside the tree the script was served from and there is no spelling that
// reaches past it; php would list whatever the host holds at that path, and
// that divergence is the point. A pattern that climbs is cleaned against the
// root, which is the rule every path a script hands this package follows.
func TestGlobIsJailedToTheSourceFilesystem(t *testing.T) {
	opts := runner.Options{RootFS: sourceTree()}
	for _, test := range []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "the host is not addressable", pattern: "/etc/host*", want: ""},
		{name: "slash is the source root", pattern: "/*", want: "/drafts,/notes.md,/one.txt,/two.txt"},
		{name: "climbing", pattern: "../*.txt", want: "one.txt,two.txt"},
		{name: "climbing twice", pattern: "drafts/../../*.txt", want: "one.txt,two.txt"},
		{name: "malformed", pattern: "[", want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := runFSOptions(t, t.TempDir(), nil, opts, `<?php echo implode(",", glob("`+test.pattern+`"));`)
			if got != test.want {
				t.Errorf("glob(%q) = %q, want %q", test.pattern, got, test.want)
			}
		})
	}
}

// TestGlobOnTheHostAnswersRelativeToTheRoot covers the branch a CLI run takes,
// where no fs.FS is bound. The matches come back from the host as paths under
// the root directory, and PHP echoes the pattern's own shape back, so the root
// has to come off again before the script sees them.
func TestGlobOnTheHostAnswersRelativeToTheRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "drafts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.txt", "two.txt", "notes.md", "drafts/three.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := runFS(t, root, nil, `<?php echo implode(",", glob("*.txt")), "|", implode(",", glob("*/*.txt"));`)
	if want := "one.txt,two.txt|drafts/three.txt"; got != want {
		t.Errorf("glob = %q, want %q", got, want)
	}
}

// TestReadsUseTheSourceFilesystemUnderANamedRoot is the regression this jail
// work exists for. The reads used to hand resolve's host path to the fs.FS,
// which rejects anything absolute, so every lookup missed and fell through to
// the host. A root that is a real directory and a source tree that is not on
// disk is the arrangement that tells the two apart.
func TestReadsUseTheSourceFilesystemUnderANamedRoot(t *testing.T) {
	opts := runner.Options{RootFS: sourceTree()}
	got := runFSOptions(t, t.TempDir(), nil, opts, `<?php
echo file_get_contents("one.txt"), ";";
echo file_get_contents("drafts/three.txt"), ";";
echo file_exists("two.txt") ? "yes" : "no", ";";
echo file_exists("missing.txt") ? "yes" : "no", ";";
echo filemtime("one.txt"), ";";
echo filemtime("missing.txt");`)
	if want := "one;three;yes;no;1756600000;0"; got != want {
		t.Errorf("reads = %q, want %q", got, want)
	}
}
