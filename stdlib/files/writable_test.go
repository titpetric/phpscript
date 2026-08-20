package files_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// writableOptions is a runner configured with an upload directory below the
// document root and a private one beside it, which is how a site that accepts
// uploads is set up.
func writableOptions() runner.Options {
	return runner.Options{WritablePaths: []string{"upload", "public/upload"}}
}

// TestWritablePathsAllowsConfiguredDirectories pins that the allowlist is a
// tree: the directory itself and anything below it.
func TestWritablePathsAllowsConfiguredDirectories(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"upload", "public/upload", "private"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	out := runFSOptions(t, root, nil, writableOptions(), `<?php
$a = fopen("upload/a.txt", "w"); fwrite($a, "a"); fclose($a);
$b = fopen("public/upload/b.txt", "w"); fwrite($b, "b"); fclose($b);
mkdir("upload/nested/deep");
$c = fopen("upload/nested/deep/c.txt", "w"); fwrite($c, "c"); fclose($c);
echo "written";`)
	if out != "written" {
		t.Fatalf("output = %q", out)
	}
	for _, name := range []string{"upload/a.txt", "public/upload/b.txt", "upload/nested/deep/c.txt"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// TestWritablePathsThrowsOutsideTheAllowlist pins the refusal and its shape: an
// exception rather than a false return, so a script cannot carry on believing
// the write happened.
func TestWritablePathsThrowsOutsideTheAllowlist(t *testing.T) {
	for _, test := range []struct {
		name string
		call string
	}{
		{name: "fopen", call: `fopen("private/x.txt", "w")`},
		{name: "fopen append", call: `fopen("private/x.txt", "a")`},
		{name: "mkdir", call: `mkdir("private/sub")`},
		{name: "unlink", call: `unlink("private/keep.txt")`},
		{name: "touch", call: `touch("private/x.txt")`},
		{name: "copy destination", call: `copy("public/index.php", "private/x.php")`},
		{name: "rename destination", call: `rename("upload/a.txt", "private/a.txt")`},
		{name: "rename source", call: `rename("private/keep.txt", "upload/a.txt")`},
		{name: "chmod", call: `chmod("private/keep.txt", 0777)`},
		{name: "escaping the root", call: `fopen("../../etc/passwd", "w")`},
		{name: "absolute path", call: `fopen("/tmp/phpscript-escape.txt", "w")`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := writableTree(t)
			out := runFSOptions(t, root, nil, writableOptions(), `<?php
try {
	`+test.call+`;
	echo "allowed";
} catch (Exception $e) {
	echo "refused: " . $e;
}`)
			if !strings.HasPrefix(out, "refused: ") {
				t.Fatalf("output = %q, want the write refused", out)
			}
			if !strings.Contains(out, "writable_paths") {
				t.Fatalf("output = %q, want the message to name writable_paths", out)
			}
			if _, err := os.Stat(filepath.Join(root, "private", "keep.txt")); err != nil {
				t.Fatalf("the refused call touched the private tree: %v", err)
			}
		})
	}
}

// Reads are not writes. An allowlist restricts what a script may change, not
// what it may look at, so the file it cannot overwrite it can still read.
func TestWritablePathsLeavesReadsAlone(t *testing.T) {
	root := writableTree(t)

	out := runFSOptions(t, root, nil, writableOptions(), `<?php
echo file_get_contents("private/keep.txt");
$f = fopen("private/keep.txt", "r");
echo "|" . stream_get_contents($f);
fclose($f);
echo "|" . (file_exists("private/keep.txt") ? "yes" : "no");`)
	if out != "kept|kept|yes" {
		t.Fatalf("output = %q", out)
	}
}

// An unconfigured allowlist is what every project that never set the key has,
// and it must keep writing wherever the user running the process may.
func TestWritablePathsUnsetAllowsEverything(t *testing.T) {
	root := writableTree(t)

	out := runFSOptions(t, root, nil, runner.Options{}, `<?php
$f = fopen("private/new.txt", "w"); fwrite($f, "ok"); fclose($f);
echo "written";`)
	if out != "written" {
		t.Fatalf("output = %q", out)
	}
}

// A prefix is not a parent: uploads-old sits beside upload, not inside it.
func TestWritablePathsRejectsASiblingSharingAPrefix(t *testing.T) {
	root := writableTree(t)
	if err := os.MkdirAll(filepath.Join(root, "upload-old"), 0o755); err != nil {
		t.Fatal(err)
	}

	out := runFSOptions(t, root, nil, writableOptions(), `<?php
try {
	$f = fopen("upload-old/x.txt", "w");
	echo "allowed";
} catch (Exception $e) {
	echo "refused";
}`)
	if out != "refused" {
		t.Fatalf("output = %q, want a sibling directory refused", out)
	}
}

// writableTree builds the layout the tests above share: two writable upload
// directories and a private one holding a file worth protecting.
func writableTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"upload", "public/upload", "private"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := map[string]string{
		"private/keep.txt": "kept",
		"upload/a.txt":     "a",
		"public/index.php": "<?php echo 'index';",
	}
	for name, content := range write {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
