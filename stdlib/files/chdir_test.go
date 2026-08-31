package files_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// workdirTree is a two-level source filesystem with a file of the same name at
// each level, which is what tells a working directory apart from a root.
func workdirTree() fstest.MapFS {
	return fstest.MapFS{
		"note.txt":         {Data: []byte("root")},
		"app/note.txt":     {Data: []byte("app")},
		"app/lib/note.txt": {Data: []byte("lib")},
		"app/boot.php":     {Data: []byte(`<?php $loaded = "app/boot.php";`)},
		"boot.php":         {Data: []byte(`<?php $loaded = "boot.php";`)},
	}
}

// runWorkdir runs src against workdirTree and returns what it echoed.
func runWorkdir(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{SAPI: "cli", RootFS: workdirTree()})
	stdlib.Register(rt)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// TestGetcwdIsWrittenFromTheSourceRoot pins the spelling php cannot be compared
// against: the source filesystem is mounted at "/", so that is where a script
// starts and what every path it reads is written from.
func TestGetcwdIsWrittenFromTheSourceRoot(t *testing.T) {
	for _, test := range []struct {
		name string
		src  string
		want string
	}{
		{name: "root", src: `<?php echo getcwd();`, want: "/"},
		{name: "one level", src: `<?php chdir("app"); echo getcwd();`, want: "/app"},
		{name: "two levels", src: `<?php chdir("app"); chdir("lib"); echo getcwd();`, want: "/app/lib"},
		{name: "written from the root", src: `<?php chdir("app"); chdir("/"); echo getcwd();`, want: "/"},
		{name: "absolute", src: `<?php chdir("/app/lib"); echo getcwd();`, want: "/app/lib"},
		{name: "back up", src: `<?php chdir("app/lib"); chdir(".."); echo getcwd();`, want: "/app"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := runWorkdir(t, test.src); got != test.want {
				t.Errorf("%s = %q, want %q", test.src, got, test.want)
			}
		})
	}
}

// TestChdirCannotLeaveTheRoot pins the jail. A path that would climb above the
// source filesystem stops at it rather than being refused, which is the rule
// every path a script hands this package follows.
func TestChdirCannotLeaveTheRoot(t *testing.T) {
	for _, src := range []string{
		`<?php chdir(".."); echo getcwd();`,
		`<?php chdir("../../.."); echo getcwd();`,
		`<?php chdir("app"); chdir("../../.."); echo getcwd();`,
		`<?php chdir("/../.."); echo getcwd();`,
	} {
		if got := runWorkdir(t, src); got != "/" {
			t.Errorf("%s = %q, want %q", src, got, "/")
		}
	}
}

// TestChdirRefusesWhatIsNotADirectory pins the false return. PHP warns and
// answers false; there is no warning channel here, so the answer is the whole
// report and the working directory has to be left where it was.
func TestChdirRefusesWhatIsNotADirectory(t *testing.T) {
	for _, test := range []struct {
		name string
		src  string
	}{
		{name: "missing", src: `<?php chdir("app"); var_dump(chdir("nope")); echo getcwd();`},
		{name: "a file", src: `<?php chdir("app"); var_dump(chdir("note.txt")); echo getcwd();`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if want := "bool(false)\n/app"; runWorkdir(t, test.src) != want {
				t.Errorf("%s = %q, want %q", test.src, runWorkdir(t, test.src), want)
			}
		})
	}
}

// TestWorkDirMovesEveryRelativePath is the reason chdir is worth having: the
// include and the read have to name the same file, or a script that moves the
// working directory gets two different answers to the same question.
func TestWorkDirMovesEveryRelativePath(t *testing.T) {
	got := runWorkdir(t, `<?php
echo file_get_contents("note.txt"), "|";
chdir("app");
include "boot.php";
echo $loaded, "|", file_get_contents("note.txt"), "|", implode(",", glob("*.txt")), "|";
chdir("lib");
echo file_get_contents("note.txt"), "|";
// A path written from the root ignores the working directory, which is what
// makes __DIR__ . "/x" name one file wherever chdir has been.
echo file_get_contents("/note.txt");`)
	if want := "root|app/boot.php|app|note.txt|lib|root"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestIncludeCacheIsKeyedByTheResolvedFile is the bug a mutable working
// directory would otherwise introduce. The cache is shared across runtimes by
// the server and the fixture harness, so a key that was the spelling rather
// than the file would hand one request the file another request loaded.
func TestIncludeCacheIsKeyedByTheResolvedFile(t *testing.T) {
	got := runWorkdir(t, `<?php
chdir("app");
include "boot.php";
echo $loaded, "|";
chdir("/");
include "boot.php";
echo $loaded;`)
	if want := "app/boot.php|boot.php"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestIncludeOnceDedupesOnTheResolvedFile is the same question for the *_once
// forms: two directories can spell one name, and only one of them has run.
func TestIncludeOnceDedupesOnTheResolvedFile(t *testing.T) {
	got := runWorkdir(t, `<?php
chdir("app");
include_once "boot.php";
echo $loaded, "|";
chdir("/");
include_once "boot.php";
echo $loaded, "|";
require_once "boot.php";
echo $loaded;`)
	if want := "app/boot.php|boot.php|boot.php"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestWorkDirDoesNotOutliveASession pins that the directory is per-request
// state. A host that reuses a runtime starts each session where it configured
// the runtime, not wherever the last script wandered to.
func TestWorkDirDoesNotOutliveASession(t *testing.T) {
	prog, err := parser.Parse(`<?php echo getcwd(), "|"; chdir("app"); echo getcwd();`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{SAPI: "cli", RootFS: workdirTree()})
	stdlib.Register(rt)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if want := "/|/app"; out.String() != want {
		t.Fatalf("first run = %q, want %q", out.String(), want)
	}

	var second strings.Builder
	rt.ResetSession(&second, nil)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if want := "/|/app"; second.String() != want {
		t.Errorf("second run = %q, want %q; the working directory outlived the session", second.String(), want)
	}
}
