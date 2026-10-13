package files_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/files"
)

// TestWithin covers the containment test a host shares with the allowlist: a
// server serving a writable directory has to know not to execute the PHP a
// script may have written into it. The separator is what keeps a sibling out,
// so a directory whose name starts with a root's name is outside it.
func TestWithin(t *testing.T) {
	for _, test := range []struct {
		name string
		dir  string
		want bool
	}{
		{name: "/srv/site/upload", dir: "/srv/site/upload", want: true},
		{name: "/srv/site/upload/a.txt", dir: "/srv/site/upload", want: true},
		{name: "/srv/site/upload/deep/a.txt", dir: "/srv/site/upload", want: true},
		{name: "/srv/site/upload-old/a.txt", dir: "/srv/site/upload", want: false},
		{name: "/srv/site/uploads", dir: "/srv/site/upload", want: false},
		{name: "/srv/site", dir: "/srv/site/upload", want: false},
		{name: "/srv/site/upload/a.txt", dir: "/srv/site/upload/", want: true},
	} {
		if got := files.Within(test.name, test.dir); got != test.want {
			t.Errorf("Within(%q, %q) = %t, want %t", test.name, test.dir, got, test.want)
		}
	}
}

// TestHostPath covers the mapping a binding in another package calls, so the
// file it opens is the file file_get_contents would open for the same
// spelling. The rule is the runtime's, and every caller joining its answer
// onto the bound root is what keeps one rule for the whole tree.
func TestHostPath(t *testing.T) {
	rt := runner.New(&strings.Builder{}, runner.Options{})
	root := filepath.Join("/srv", "site")

	for _, test := range []struct {
		path string
		want string
	}{
		{path: "", want: ""},
		{path: "index.php", want: filepath.Join(root, "index.php")},
		{path: "/index.php", want: filepath.Join(root, "index.php")},
		{path: "a/../b.php", want: filepath.Join(root, "b.php")},
		{path: "../../etc/passwd", want: filepath.Join(root, "etc", "passwd")},
	} {
		if got := files.HostPath(rt, root, test.path); got != test.want {
			t.Errorf("HostPath(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

// TestWritableRoots covers how a configured writable_paths entry is read: a
// relative entry is a path inside the project and resolves under the bound
// root, and an absolute one is kept as written and then matches nothing,
// because the path it is compared against has already been clamped into the
// root. There is no spelling that grants a write outside the tree.
func TestWritableRoots(t *testing.T) {
	root := filepath.Join("/srv", "site")

	for _, test := range []struct {
		name  string
		paths []string
		want  []string
	}{
		{name: "nothing configured", paths: nil, want: nil},
		{name: "blank entries are dropped", paths: []string{"", "  "}, want: []string{}},
		{
			name:  "a relative entry resolves under the root",
			paths: []string{"upload", "public/upload"},
			want:  []string{filepath.Join(root, "upload"), filepath.Join(root, "public", "upload")},
		},
		{
			name:  "a climbing entry is clamped to the root",
			paths: []string{"../../etc"},
			want:  []string{filepath.Join(root, "etc")},
		},
		{
			name:  "an absolute entry is kept as written",
			paths: []string{"/srv/shared"},
			want:  []string{filepath.FromSlash("/srv/shared")},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := files.WritableRoots(root, test.paths)
			if len(got) != len(test.want) {
				t.Fatalf("WritableRoots(%v) = %v, want %v", test.paths, got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Errorf("root %d = %q, want %q", i, got[i], test.want[i])
				}
			}
		})
	}

	// An absolute entry grants nothing: the path a script's /srv/shared
	// resolves to is under the root, so it is never inside /srv/shared.
	if allowed := files.Within(files.HostPath(runner.New(&strings.Builder{}, runner.Options{}), root, "/srv/shared/x"), filepath.FromSlash("/srv/shared")); allowed {
		t.Error("an absolute writable_paths entry allowed a write")
	}
}

// TestRegister covers the root Register binds and the names it installs. The
// root is the process working directory, the one a CLI run reads and writes.
func TestRegister(t *testing.T) {
	rt := runner.New(&strings.Builder{}, runner.Options{})
	if rt.FunctionExists("file_get_contents") {
		t.Fatal("a bare runtime answers for file_get_contents before anything registered it")
	}

	files.Register(rt)
	for _, name := range []string{
		"file_get_contents", "file_put_contents", "fopen", "fwrite", "fclose",
		"is_file", "is_dir", "scandir", "pathinfo", "realpath",
		"fread", "fgets", "feof", "fseek", "ftell", "rewind",
		"mkdir", "unlink", "copy", "rename", "touch", "chmod",
		"getcwd", "chdir",
	} {
		if !rt.FunctionExists(name) {
			t.Errorf("%s does not resolve after Register", name)
		}
	}
}

// TestRegisterRoot covers what a host binds a project directory with: it
// replaces whatever root was installed before it, so a path a script writes
// resolves under the directory named last and never under the first.
func TestRegisterRoot(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()

	program, err := parser.Parse(`<?php file_put_contents("written.txt", "data"); echo "ok";`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	files.RegisterRoot(rt, first)
	files.RegisterRoot(rt, second)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != "ok" {
		t.Fatalf("output = %q", out.String())
	}

	if _, err := os.Stat(filepath.Join(second, "written.txt")); err != nil {
		t.Errorf("the write did not land under the root bound last: %v", err)
	}
	if _, err := os.Stat(filepath.Join(first, "written.txt")); err == nil {
		t.Error("the write landed under the root that was replaced")
	}
}

// writableOptions is a runner configured with an upload directory below the
// document root and a private one beside it, so a site that accepts
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
file_put_contents("upload/d.txt", "d");
echo "written";`)
	if out != "written" {
		t.Fatalf("output = %q", out)
	}
	for _, name := range []string{"upload/a.txt", "public/upload/b.txt", "upload/nested/deep/c.txt", "upload/d.txt"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// TestWritablePathsThrowsOutsideTheAllowlist pins the refusal and its shape: an
// exception and no false return, so a script cannot carry on believing
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
		{name: "file_put_contents", call: `file_put_contents("private/x.txt", "data")`},
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
