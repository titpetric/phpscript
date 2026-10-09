package runner_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// runScript runs src on an interpreter runtime with the standard library
// registered and returns what the script printed with the run error. entry names
// a declared function to call after the file has run, for a source that declares
// and executes nothing.
func runScript(t *testing.T, src, entry string) (string, error) {
	t.Helper()
	program, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	if runErr := rt.Run(program); runErr != nil {
		return out.String(), runErr
	}
	if entry == "" {
		return out.String(), nil
	}
	_, runErr := rt.InvokeNamed(entry)
	return out.String(), runErr
}

// A first-class callable resolves the same names a call resolves, so a host
// static registered under its whole spelling answers, `self::` resolves against
// the method it was written in, and a bound method keeps its receiver.
func TestFirstClassCallableResolution(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		want  string
		entry string
	}{
		{
			name: "host static registered under the whole spelling",
			src:  `<?php $f = Closure::fromCallable(...); $g = $f("strtoupper"); echo $g("ab");`,
			want: "AB",
		},
		{
			name: "self names the enclosing class",
			src:  `<?php class A { function pick() { return self::shout(...); } static function shout($n) { return "S" . $n; } } $a = new A(); $f = $a->pick(); echo $f("1");`,
			want: "S1",
		},
		{
			name: "a bound method keeps its receiver",
			src:  `<?php class B { public $p = "b"; function read() { return $this->p; } } $b = new B(); $f = $b->read(...); $b->p = "changed"; echo $f();`,
			want: "changed",
		},
		{
			// An unqualified name inside a namespace resolves there first and then
			// globally, the fallback a call takes. A namespaced file may only
			// declare, so the callable is taken inside a function.
			name:  "a namespaced name falls back to the global function",
			src:   "<?php\nnamespace App;\n\nfunction pick() { $f = strtoupper(...); echo $f(\"ab\"); }\n",
			want:  "AB",
			entry: "App\\pick",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := runScript(t, test.src, test.entry)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != test.want {
				t.Fatalf("output = %q, want %q", out, test.want)
			}
		})
	}
}

// A first-class callable that names nothing reports it where the value is taken
// and not where it would have been called, with the message the equivalent
// call reports.
func TestFirstClassCallableErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "undefined function",
			src:  `<?php $f = nosuch(...);`,
			want: "call to undefined function nosuch()",
		},
		{
			name: "undefined method on a receiver",
			src:  `<?php class A { function m() {} } $a = new A(); $f = $a->nosuch(...);`,
			want: "call to undefined method A::nosuch()",
		},
		{
			name: "undefined method on a class",
			src:  `<?php class A { function m() {} } $f = A::nosuch(...);`,
			want: "call to undefined method A::nosuch()",
		},
		{
			name: "undeclared class",
			src:  `<?php $f = Nope::m(...);`,
			want: `class "Nope" not found`,
		},
		{
			name: "a value that is not callable",
			src:  `<?php $z = 5; $f = $z(...);`,
			want: "value of type int is not callable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runScript(t, test.src, "")
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want one containing %q", err, test.want)
			}
		})
	}
}
