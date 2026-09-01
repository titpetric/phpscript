package files_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// walkSrc is the recursive listing a php script writes with the handle
// functions: open, read until false, close, skipping "." and ".." by hand. The
// fixture at tests/fixtures/stdlib/opendir.phpt runs the same shape against a
// committed tree; this runs it against both filesystems.
const walkSrc = `<?php
function walk(string $dir): void {
    $handle = opendir($dir);
    if ($handle === false) { echo "!", $dir, ";"; return; }
    $names = [];
    while (($name = readdir($handle)) !== false) {
        if ($name === "." || $name === "..") { continue; }
        $names[] = $name;
    }
    closedir($handle);
    sort($names);
    foreach ($names as $name) {
        $path = $dir . "/" . $name;
        if (is_dir($path)) { echo $name, "/;"; walk($path); continue; }
        echo $name, ";";
    }
}
walk(".");`

// TestOpendirWalksTheSourceFilesystem is the branch an embedded application
// takes. There is nothing on disk to open a descriptor on, so a walk that
// answers at all proves the handle carries the listing rather than a file.
func TestOpendirWalksTheSourceFilesystem(t *testing.T) {
	opts := runner.Options{RootFS: sourceTree()}
	got := runFSOptions(t, t.TempDir(), nil, opts, walkSrc)
	if want := "drafts/;three.txt;notes.md;one.txt;two.txt;"; got != want {
		t.Errorf("walk = %q, want %q", got, want)
	}
}

// TestOpendirWalksTheHost covers the branch a CLI run takes, where no fs.FS is
// bound and the same names come off the host under the bound root.
func TestOpendirWalksTheHost(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "drafts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.txt", "two.txt", "notes.md", "drafts/three.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := runFS(t, root, nil, walkSrc)
	if want := "drafts/;three.txt;notes.md;one.txt;two.txt;"; got != want {
		t.Errorf("walk = %q, want %q", got, want)
	}
}

// TestReaddirListsDotAndDotDot pins what readdir hands back that scandir also
// hands back: php lists the two directory entries, and a script that does not
// skip them recurses forever.
func TestReaddirListsDotAndDotDot(t *testing.T) {
	opts := runner.Options{RootFS: sourceTree()}
	got := runFSOptions(t, t.TempDir(), nil, opts, `<?php
$handle = opendir("/");
$names = [];
while (($name = readdir($handle)) !== false) { $names[] = $name; }
closedir($handle);
echo implode(",", $names);`)
	if want := ".,..,drafts,notes.md,one.txt,two.txt"; got != want {
		t.Errorf("readdir = %q, want %q", got, want)
	}
}

// TestOpendirRefusesWhatIsNotADirectory pins the false return. A file and a
// name that is not there answer the same way, as php's opendir does.
func TestOpendirRefusesWhatIsNotADirectory(t *testing.T) {
	opts := runner.Options{RootFS: sourceTree()}
	got := runFSOptions(t, t.TempDir(), nil, opts, `<?php
var_dump(opendir("one.txt"));
var_dump(opendir("missing"));`)
	if want := "bool(false)\nbool(false)\n"; got != want {
		t.Errorf("opendir = %q, want %q", got, want)
	}
}

// TestClosedirExhaustsTheHandle states what a closed handle answers. There is
// no descriptor to invalidate, so closedir drops the listing and a further read
// reports the end of it; php throws a TypeError there. A value readdir was
// never handed answers the same way, which is what keeps a foreign one from
// walking anything.
func TestClosedirExhaustsTheHandle(t *testing.T) {
	opts := runner.Options{RootFS: sourceTree()}
	got := runFSOptions(t, t.TempDir(), nil, opts, `<?php
$handle = opendir("/");
closedir($handle);
var_dump(readdir($handle));
var_dump(readdir("not a handle"));`)
	if want := "bool(false)\nbool(false)\n"; got != want {
		t.Errorf("readdir after closedir = %q, want %q", got, want)
	}
}

// TestOpendirIsJailedToTheRoot holds the handle functions to the rule every
// path in this package follows: "/" is the source filesystem's root, and a path
// that climbs is cleaned against it rather than escaping.
func TestOpendirIsJailedToTheRoot(t *testing.T) {
	opts := runner.Options{RootFS: sourceTree()}
	got := runFSOptions(t, t.TempDir(), nil, opts, `<?php
var_dump(opendir("/etc") === false);
$handle = opendir("drafts/../..");
$names = [];
while (($name = readdir($handle)) !== false) { $names[] = $name; }
closedir($handle);
echo implode(",", $names);`)
	if want := "bool(true)\n.,..,drafts,notes.md,one.txt,two.txt"; got != want {
		t.Errorf("jailed opendir = %q, want %q", got, want)
	}
}
