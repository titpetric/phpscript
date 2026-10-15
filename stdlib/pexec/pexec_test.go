package pexec_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/pexec"
)

// bindings are every name one call installs: the four that run a command, the
// two that quote for it, and the two that name the running process.
var bindings = []string{
	"exec", "system", "passthru", "shell_exec",
	"escapeshellarg", "escapeshellcmd",
	"posix_getpid", "getmypid",
}

// run executes src against a runtime carrying the standard library and this
// package's bindings rooted at dir, and returns what the script printed. The
// standard library is installed because a script reading $output back reaches
// for count() and implode() to do it.
func run(t *testing.T, dir, src string) string {
	t.Helper()
	program, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	pexec.RegisterRoot(rt, dir)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// TestRegister covers the names installed and the root they start a command
// in. Register is RegisterRoot with no directory, so a command runs where the
// process runs, the directory a CLI invocation reads and writes in.
func TestRegister(t *testing.T) {
	t.Parallel()
	rt := runner.New(&strings.Builder{}, runner.Options{})
	if rt.FunctionExists("shell_exec") {
		t.Fatal("a bare runtime answers for shell_exec before anything registered it")
	}

	pexec.Register(rt)
	for _, name := range bindings {
		if !rt.FunctionExists(name) {
			t.Errorf("%s does not resolve after Register", name)
		}
	}

	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(run(t, "", `<?php echo shell_exec("pwd");`)); got != working {
		t.Errorf("a command started in %q, want the process directory %q", got, working)
	}
}

// TestRegisterRoot covers the directory a command starts in when a host bound
// one: a server serving scripts out of a project directory binds that, so
// `pwd` inside a script answers the same thing getcwd() does. It replaces
// whatever root stands before it, so a host calls it after Register.
func TestRegisterRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(run(t, dir, `<?php echo shell_exec("pwd");`)); got != resolved {
		t.Errorf("a command started in %q, want the bound root %q", got, resolved)
	}

	// The second call wins, so a host that bound a project after Register is
	// the one a command obeys.
	program, err := parser.Parse(`<?php echo shell_exec("pwd");`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	pexec.Register(rt)
	pexec.RegisterRoot(rt, dir)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != resolved {
		t.Errorf("after a second RegisterRoot a command started in %q, want %q", got, resolved)
	}
}

// TestRegisterNamesTheRunningProcess covers posix_getpid and getmypid: both
// name the interpreter and never a command it starts, so the two report the
// same number and it is the test binary's own.
func TestRegisterNamesTheRunningProcess(t *testing.T) {
	t.Parallel()
	want := strconv.Itoa(os.Getpid())
	if got := run(t, "", `<?php echo posix_getpid();`); got != want {
		t.Errorf("posix_getpid() = %q, want %q", got, want)
	}
	if got := run(t, "", `<?php echo getmypid();`); got != want {
		t.Errorf("getmypid() = %q, want %q", got, want)
	}
}
