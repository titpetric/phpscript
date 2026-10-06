package stdlib_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// pexecNames is everything stdlib/pexec registers: the Exec profile area in
// full. A name added to that package belongs in this list, so the secure
// profile keeps excluding it.
var pexecNames = []string{
	"exec", "system", "passthru", "shell_exec",
	"escapeshellarg", "escapeshellcmd",
	"posix_getpid", "getmypid",
}

// mountScript parses and runs src on a runtime mounted at profile, on the
// interpreter or the bytecode engine, returning the output and the run error.
func mountScript(t *testing.T, profile stdlib.Profile, flat bool, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	if flat {
		rt = runner.NewFlatStack(&out, runner.Options{})
	}
	stdlib.Mount(rt, profile)
	err = rt.Run(prog)
	return out.String(), err
}

// listPexec is a script printing every pexec name the runtime defines, "none"
// when it defines none.
func listPexec() string {
	return `<?php
foreach (array("` + strings.Join(pexecNames, `", "`) + `") as $name) {
	if (function_exists($name)) { echo $name, " "; }
}
echo "none";`
}

// TestMountSecureExcludesExec pins the security property of the secure
// profile: no pexec name is defined, and no call route reaches one - a direct
// call, call_user_func and a variable function all resolve through the one
// function table, on either engine. strlen answers beside each refusal, so the
// failure is the absence of the name rather than a broken runtime.
func TestMountSecureExcludesExec(t *testing.T) {
	calls := []struct {
		name string
		php  string
		want string
	}{
		{"function_exists", listPexec(), "none"},
		{"direct call", `<?php echo strlen("abc"); exec("echo hi");`, "undefined function exec"},
		{"call_user_func", `<?php echo strlen("abc"); call_user_func("shell_exec", "echo hi");`, "must be a valid callback"},
		{"variable function", `<?php echo strlen("abc"); $fn = "system"; $fn("echo hi");`, "not callable"},
	}
	for _, engine := range []struct {
		name string
		flat bool
	}{{"interpreter", false}, {"flatstack", true}} {
		t.Run(engine.name, func(t *testing.T) {
			for _, tc := range calls {
				t.Run(tc.name, func(t *testing.T) {
					out, err := mountScript(t, stdlib.Secure, engine.flat, tc.php)
					if tc.name == "function_exists" {
						if err != nil {
							t.Fatalf("run: %v", err)
						}
						if out != tc.want {
							t.Fatalf("defined under Secure: got %q, want %q", out, tc.want)
						}
						return
					}
					if out != "3" {
						t.Fatalf("secure surface broken: strlen printed %q", out)
					}
					if err == nil || !strings.Contains(err.Error(), tc.want) {
						t.Fatalf("want error containing %q, got %v", tc.want, err)
					}
				})
			}
		})
	}
}

// TestMountExecEnablesExec pins the other half of the bitmask: a profile
// naming Exec defines every pexec name and a command actually runs.
func TestMountExecEnablesExec(t *testing.T) {
	out, err := mountScript(t, stdlib.Secure|stdlib.Exec, false, listPexec())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := strings.Join(pexecNames, " ") + " none"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	out, err = mountScript(t, stdlib.Secure|stdlib.Exec, false, `<?php echo trim(shell_exec("echo mounted"));`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "mounted" {
		t.Fatalf("shell_exec under Secure|Exec printed %q", out)
	}
}

// TestRegisterKeepsFullSurface pins what the CLI, the fixture runner and the
// demos rely on: Register is every profile area, Exec included.
func TestRegisterKeepsFullSurface(t *testing.T) {
	if got, want := runScript(t, listPexec()), strings.Join(pexecNames, " ")+" none"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestDefaultProfileIsSecure pins the constant the configuration story rests
// on: the default profile for an untrusted host names no insecure area.
func TestDefaultProfileIsSecure(t *testing.T) {
	if stdlib.Default != stdlib.Secure {
		t.Fatalf("stdlib.Default = %b, want stdlib.Secure (%b)", stdlib.Default, stdlib.Secure)
	}
	if stdlib.Default&stdlib.Exec != 0 {
		t.Fatal("stdlib.Default names Exec")
	}
}
