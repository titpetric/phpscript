package files_test

import (
	"os"
	"path/filepath"
	"testing"
)

// statRoot builds a project holding one file, one directory; a script asking
// about a path has both kinds to ask about.
func statRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pages", "index.php"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestStatAnswersForBothKinds covers the stat family's report against php's
// output for the same paths: is_file is false for a directory, is_dir is false
// for a file, filesize is the byte count, and filetype names the kind. A
// missing path reports false from the last two.
func TestStatAnswersForBothKinds(t *testing.T) {
	t.Parallel()
	out := runFS(t, statRoot(t), nil, `<?php
echo "[", is_file("pages/index.php") ? "y" : "n", is_file("pages") ? "y" : "n", "]";
echo "[", is_dir("pages") ? "y" : "n", is_dir("pages/index.php") ? "y" : "n", "]";
echo "[", filesize("pages/index.php"), "]";
echo "[", filetype("pages/index.php"), ",", filetype("pages"), "]";
echo "[", filesize("missing.php") === false ? "false" : "other", ",", filetype("missing.php") === false ? "false" : "other", "]";`)
	if want := "[yn][yn][5][file,dir][false,false]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestStatResolvesThroughTheBoundRoot covers the rule this family shares with
// file_get_contents: a path written from "/" names the bound root; the host
// root is out of reach. A relative path resolves against the working directory. A second
// resolver written here would name a second file for one spelling, a spelling
// no sandbox can permit.
func TestStatResolvesThroughTheBoundRoot(t *testing.T) {
	t.Parallel()
	out := runFS(t, statRoot(t), nil, `<?php
echo "[", is_file("/pages/index.php") ? "y" : "n", "]";
echo "[", is_dir("/") ? "y" : "n", "]";
chdir("pages");
echo "[", is_file("index.php") ? "y" : "n", "]";
echo "[", is_file("/etc/passwd") ? "y" : "n", "]";
echo "[", is_file("../../../../etc/passwd") ? "y" : "n", "]";`)
	if want := "[y][y][y][n][n]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestScandirListsTheDotEntries covers php's scandir output: the names sorted,
// including "." and "..", because PHP reports the directory the operating
// system reports. The call filters nothing.
func TestScandirListsTheDotEntries(t *testing.T) {
	t.Parallel()
	out := runFS(t, statRoot(t), nil, `<?php echo implode(",", scandir("pages"));`)
	if want := ".,..,index.php"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestPathinfoSplitsAPath covers each key php returns. A path that has no
// extension returns no extension key at all and isset() on it answers false,
// as in php.
func TestPathinfoSplitsAPath(t *testing.T) {
	t.Parallel()
	out := runFS(t, statRoot(t), nil, `<?php
$info = pathinfo("a/b/c.txt");
echo "[", $info["dirname"], ",", $info["basename"], ",", $info["extension"], ",", $info["filename"], "]";
$bare = pathinfo("a/b/c");
echo "[", isset($bare["extension"]) ? "set" : "unset", ",", $bare["filename"], "]";`)
	if want := "[a/b,c.txt,txt,c][unset,c]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestRealpathAnswersInsideTheRoot covers the path spelling realpath reports:
// a script can only address the bound tree and the path it reports starts at
// that root and holds no host prefix a script could write back out.
func TestRealpathAnswersInsideTheRoot(t *testing.T) {
	t.Parallel()
	out := runFS(t, statRoot(t), nil, `<?php
echo "[", realpath("pages/../pages/index.php"), "]";
echo "[", realpath("missing.php") === false ? "false" : "other", "]";`)
	if want := "[/pages/index.php][false]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}
