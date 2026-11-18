package stdlib_test

import (
	"io"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// pexecNames is everything stdlib/ps/pexec registers: the Exec profile area in
// full. A name added to that package belongs in this list and the secure
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

// TestMountSecureExcludesExec pins the secure profile's security property: it
// defines no pexec name. No call route reaches one. A direct call,
// call_user_func, or a variable function all resolve via one function table, on
// both engines. strlen answers beside each refusal. The failure is the name's
// absence; the runtime still works.
// TestMountGoBindsHeader covers the request function the go runtime installs.
func TestMountGoBindsHeader(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{Runtime: "go"})
	stdlib.Mount(rt, stdlib.Secure)
	for _, name := range []string{"header", "http_response_code", "getallheaders"} {
		if _, ok := rt.LookupFunc(name); !ok {
			t.Errorf("the go runtime installed no %s", name)
		}
	}
}

func TestMountSecureExcludesExec(t *testing.T) {
	t.Parallel()
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
// naming Exec defines every pexec name. A command then runs.
func TestMountExecEnablesExec(t *testing.T) {
	t.Parallel()
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

// TestRegisterKeepsFullSurface pins the CLI, the fixture runner, and the demos'
// expectation: Register covers every profile area, Exec included.
func TestRegisterKeepsFullSurface(t *testing.T) {
	t.Parallel()
	if got, want := runScript(t, listPexec()), strings.Join(pexecNames, " ")+" none"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestRegisterFSKeepsTheProfile covers the reroot. RegisterFS reinstalls the
// areas it reroots, and rerooting pexec unconditionally handed back every name
// the secure mount had just left out. Every mount site calls Mount and
// RegisterFS in that order. The mask reached nothing.
func TestRegisterFSKeepsTheProfile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		profile stdlib.Profile
		want    string
	}{
		{"secure", stdlib.Secure, "none"},
		{"secure|exec", stdlib.Secure | stdlib.Exec, strings.Join(pexecNames, " ") + " none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := parser.Parse(listPexec())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var out strings.Builder
			rt := runner.New(&out, runner.Options{})
			stdlib.Mount(rt, tc.profile)
			stdlib.RegisterFS(rt, ".")
			if err := rt.Run(prog); err != nil {
				t.Fatalf("run: %v", err)
			}
			if out.String() != tc.want {
				t.Fatalf("after RegisterFS: got %q, want %q", out.String(), tc.want)
			}
		})
	}
}

// TestMountSelectedAreas covers the default runtime's configuration: a runtime
// built with Options.Areas installs the areas below those namespaces alone.
// One built without the option installs every area.
func TestMountSelectedAreas(t *testing.T) {
	narrow := runner.New(io.Discard, runner.Options{Areas: []string{`Runtime\PS\Core\Strings`}})
	stdlib.Register(narrow)
	if !narrow.FunctionExists("strlen") {
		t.Error("the selected area is missing strlen")
	}
	for _, name := range []string{"array_map", "json_encode", "ob_start"} {
		if narrow.FunctionExists(name) {
			t.Errorf("%s is installed outside the selected area", name)
		}
	}

	stdlib.RegisterFS(narrow, ".")
	for _, name := range []string{"file_get_contents", "fopen", "unlink"} {
		if narrow.FunctionExists(name) {
			t.Errorf("RegisterFS defined %s outside the selected area", name)
		}
	}

	full := runner.New(io.Discard, runner.Options{})
	stdlib.Register(full)
	for _, name := range []string{"strlen", "array_map", "json_encode", "ob_start"} {
		if !full.FunctionExists(name) {
			t.Errorf("the default runtime lacks %s", name)
		}
	}
}
