package runner_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// runRedeclare runs src with the standard library installed and returns the
// output and the error, since both matter here: the error is the point, and the
// output says how far the program got before it.
func runRedeclare(t *testing.T, src string, files fstest.MapFS) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{SAPI: "cli", RootFS: files})
	stdlib.Register(rt)
	// Run before reading the buffer: a return statement evaluates its
	// operands left to right, so returning both in one expression would
	// report the output as it was before the program wrote any.
	runErr := rt.Run(prog)
	return out.String(), runErr
}

// TestRedeclaringAFunctionFails pins the two shapes of the refusal and their
// messages, which are php's own text.
func TestRedeclaringAFunctionFails(t *testing.T) {
	for _, test := range []struct {
		name string
		src  string
		want string
	}{
		{
			// A program parsed straight from a string has no
			// entrypoint to name, so the site falls back to saying
			// there was one rather than pointing at an empty path.
			name: "twice in one file",
			src:  "<?php\nfunction a() { return 1; }\nfunction a() { return 2; }\n",
			want: "Cannot redeclare function a() (previously declared in an earlier declaration)",
		},
		{
			name: "over a registered binding",
			src:  "<?php\nfunction strlen($s) { return 0; }\n",
			want: "Cannot redeclare function strlen()",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := runRedeclare(t, test.src, nil)
			if err == nil {
				t.Fatal("run returned nil error")
			}
			if err.Error() != test.want {
				t.Errorf("error = %q, want %q", err, test.want)
			}
			var redeclared *runner.RedeclareError
			if !errors.As(err, &redeclared) {
				t.Fatalf("error is %T, want *runner.RedeclareError", err)
			}
			// Exception rather than a name of its own, so the clause a
			// script would write for it is the clause that takes it.
			if got := redeclared.ThrowableClass(); got != "Exception" {
				t.Errorf("ThrowableClass() = %q, want %q", got, "Exception")
			}
		})
	}
}

// TestRedeclaringThroughAnIncludeIsCatchable is the case that reaches a script
// rather than the host: an include hoists inside the script's own flow, so the
// try around it takes the error and the program carries on.
func TestRedeclaringThroughAnIncludeIsCatchable(t *testing.T) {
	files := fstest.MapFS{
		"dup.php": {Data: []byte("<?php\nfunction dup() { return 1; }\nfunction dup() { return 2; }\n")},
	}
	out, err := runRedeclare(t, `<?php
try {
	include "dup.php";
} catch (Exception $e) {
	echo "caught: ", get_class($e), ": ", $e->getMessage();
}
`, files)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "caught: Exception: Cannot redeclare function dup() (previously declared in /dup.php:2) in /dup.php on line 3"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestRedeclareNamesTheWholePathBelowTheRoot pins the spelling of the file in
// the message. php prints host paths; a runtime whose scripts may come out of
// an fs.FS has none to print, so the path is the one the script named, resolved
// against the root. What it must not do is shorten to a basename: two files
// called helpers.php in different folders is exactly the case the message
// exists to tell apart.
func TestRedeclareNamesTheWholePathBelowTheRoot(t *testing.T) {
	files := fstest.MapFS{
		"a/b/c/deep.php": {Data: []byte("<?php\n\nfunction deepfn() { return 1; }\n\nfunction deepfn() { return 2; }\n")},
	}
	_, err := runRedeclare(t, `<?php
include "a/b/c/deep.php";
`, files)
	if err == nil {
		t.Fatal("run returned nil error")
	}
	want := "Cannot redeclare function deepfn() (previously declared in /a/b/c/deep.php:3) in /a/b/c/deep.php on line 5"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

// TestDeclaringTheSameFunctionAfterResetSucceeds pins the session boundary. The
// fixture harness and any host that reuses a runtime run the same program more
// than once through it, and the second run has to be a fresh slate rather than
// a collision with the first.
func TestDeclaringTheSameFunctionAfterResetSucceeds(t *testing.T) {
	src := "<?php\nfunction reused() { return 7; }\necho reused();"
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var first strings.Builder
	rt := runner.New(&first, runner.Options{SAPI: "cli"})
	stdlib.Register(rt)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("first run: %v", err)
	}

	var second strings.Builder
	rt.ResetSession(&second, nil)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if first.String() != "7" || second.String() != "7" {
		t.Errorf("output = %q then %q, want %q twice", first.String(), second.String(), "7")
	}
	// The reset takes the closure out of the function table too, not just
	// out of the name set, so function_exists does not answer for a function
	// the new session has not declared yet.
	rt.ResetSession(&second, nil)
	if rt.FunctionExists("reused") {
		t.Error("FunctionExists(reused) is true after a reset that did not declare it")
	}
}
