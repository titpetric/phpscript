package files_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/ps/files"
)

// TestWithin covers the containment test a host shares with the allowlist: a
// server serving a writable directory must avoid executing PHP a script wrote
// into it. The separator keeps a sibling out. A directory whose name starts with
// a root's name lies outside the root.
func TestWithin(t *testing.T) {
	t.Parallel()
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

// TestHostPath covers the mapping a binding in another package calls. The file
// it opens is the file file_get_contents would open for the same spelling. The
// rule belongs to the runtime. A caller joining that path onto the bound root
// keeps one rule for the whole tree.
func TestHostPath(t *testing.T) {
	t.Parallel()
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

// TestWritableRoots covers how WritableRoots reads a configured
// writable_paths entry: a relative entry is a path inside the project and
// resolves beneath the bound root. An absolute entry survives as written, then
// matches nothing, because resolve clamps the compared path into the root.
// There is no spelling that grants a write outside the tree.
func TestWritableRoots(t *testing.T) {
	t.Parallel()
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

	// An absolute entry grants no access: the path a script's /srv/shared
	// resolves to lies beneath the root, never landing inside /srv/shared.
	if allowed := files.Within(files.HostPath(runner.New(&strings.Builder{}, runner.Options{}), root, "/srv/shared/x"), filepath.FromSlash("/srv/shared")); allowed {
		t.Error("an absolute writable_paths entry allowed a write")
	}
}

// TestRegister covers the root Register binds and the names it installs. The
// root is the process working directory. A CLI run reads and writes that
// directory.
func TestRegister(t *testing.T) {
	t.Parallel()
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

// TestRegisterRoot covers the directory a host binds a project to. RegisterRoot
// replaces any earlier root. A path a script writes resolves beneath the
// directory named last, never beneath the first.
func TestRegisterRoot(t *testing.T) {
	t.Parallel()
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

// writableOptions returns a runner configured with an upload directory below
// the document root, plus a private directory alongside. That layout matches a
// site accepting uploads.
func writableOptions() runner.Options {
	return runner.Options{WritablePaths: []string{"upload", "public/upload"}}
}

// TestWritablePathsAllowsConfiguredDirectories pins the allowlist as a tree: the directory and anything beneath.
func TestWritablePathsAllowsConfiguredDirectories(t *testing.T) {
	t.Parallel()
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
// exception, no false return. A script cannot continue believing the write
// happened.
func TestWritablePathsThrowsOutsideTheAllowlist(t *testing.T) {
	t.Parallel()
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

// Reads are not writes. An allowlist restricts the changes a script may make. It
// does not restrict reads: a file it cannot overwrite is still readable.
func TestWritablePathsLeavesReadsAlone(t *testing.T) {
	t.Parallel()
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

// A project without a key setting leaves the allowlist unconfigured. It must
// keep writing wherever the process user may write.
func TestWritablePathsUnsetAllowsEverything(t *testing.T) {
	t.Parallel()
	root := writableTree(t)

	out := runFSOptions(t, root, nil, runner.Options{}, `<?php
$f = fopen("private/new.txt", "w"); fwrite($f, "ok"); fclose($f);
echo "written";`)
	if out != "written" {
		t.Fatalf("output = %q", out)
	}
}

// A prefix is not a parent: upload-old is a sibling of upload.
func TestWritablePathsRejectsASiblingSharingAPrefix(t *testing.T) {
	t.Parallel()
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
// directories, plus a private directory holding a file worth protecting.
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
