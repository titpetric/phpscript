package runner_test

import (
	"io"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// callableDeclarations declares one of each spelling AsCallable has an answer
// for, and leaves the value under a constant so a Go test can read it back.
const callableDeclarations = `<?php

class Site {
	public $greeting = "hei";

	public function hello($who) { return $this->greeting . " " . $who; }

	public static function shout($who) { return "HELLO " . $who; }

	public function __invoke($who) { return "invoked " . $who; }
}

function plain($who) { return "plain " . $who; }

$site = new Site;
define("CLOSURE", function ($who) { return "closure " . $who; });
define("BOUND", $site->hello);
define("SITE", $site);
define("ARR", array($site, "hello"));
define("FC_FUNC", plain(...));
define("FC_METHOD", $site->hello(...));
define("FC_STATIC", Site::shout(...));
`

// callableRuntime runs the declarations and answers the runtime holding them.
func callableRuntime(t testing.TB) *runner.Runtime {
	t.Helper()
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)
	program, err := rt.Load(callableDeclarations)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	return rt
}

// constant reads a value the declarations left behind.
func constant(t testing.TB, rt *runner.Runtime, name string) any {
	t.Helper()
	value, ok := rt.Const(name)
	if !ok {
		t.Fatalf("%s was not defined", name)
	}
	return value
}

// TestAsCallableTakesEverySpellingButAnArray is the host-facing half of the
// callable contract: AsCallable answers a value another runtime can run, so it
// needs a declaration to run and refuses anything that is not one.
//
// The two array spellings are the documented refusal (docs/README.md): they stay
// callable everywhere else. Class::method is not one of them and used to be
// refused as "no PHP function of that name is declared", which was untrue of a
// method the class declares.
func TestAsCallableTakesEverySpellingButAnArray(t *testing.T) {
	rt := callableRuntime(t)

	accepted := []struct {
		name  string
		value any
		want  string
		named string
	}{
		{name: "closure", value: constant(t, rt, "CLOSURE"), want: "closure a", named: "closure"},
		{name: "bound method", value: constant(t, rt, "BOUND"), want: "hei a", named: "Site::hello"},
		{name: "function name", value: "plain", want: "plain a", named: "plain"},
		{name: "Class::method", value: "Site::shout", want: "HELLO a", named: "Site::shout"},
		{name: "an object with __invoke", value: constant(t, rt, "SITE"), want: "invoked a", named: "Site::__invoke"},
		// The first-class spellings arrive as a *Callable already, so the router
		// takes the form php itself writes rather than only the string for it.
		{name: "name(...)", value: constant(t, rt, "FC_FUNC"), want: "plain a", named: "plain"},
		{name: "$obj->method(...)", value: constant(t, rt, "FC_METHOD"), want: "hei a", named: "Site::hello"},
		{name: "Class::method(...)", value: constant(t, rt, "FC_STATIC"), want: "HELLO a", named: "Site::shout"},
	}
	for _, test := range accepted {
		t.Run(test.name, func(t *testing.T) {
			callable, err := rt.AsCallable(test.value)
			if err != nil {
				t.Fatalf("AsCallable: %v", err)
			}
			if got := callable.Name(); got != test.named {
				t.Errorf("Name() = %q, want %q", got, test.named)
			}
			result, err := callable.Call("a")
			if err != nil {
				t.Fatalf("Call: %v", err)
			}
			if got, _ := result.(string); got != test.want {
				t.Errorf("Call() = %v, want %q", result, test.want)
			}
		})
	}

	refused := []struct {
		name  string
		value any
		want  string
	}{
		{
			name:  "array with an object",
			value: constant(t, rt, "ARR"),
			want:  `the array($object, "method") spelling is not a handler here`,
		},
		{name: "an int", value: int64(42), want: "a callable is a closure"},
		{name: "nothing", value: nil, want: "a callable is a closure"},
		{name: "an empty name", value: "", want: "the handler name is empty"},
		{name: "a name nothing declares", value: "nope", want: "no PHP function of that name is declared"},
		{name: "a class nothing declares", value: "Nope::shout", want: "no class of that name is declared"},
		{name: "a method the class lacks", value: "Site::nope", want: "the class declares no method of that name"},
		{
			name:  "a Go func",
			value: func(...any) (any, error) { return nil, nil },
			want:  "a callable is a closure",
		},
	}
	for _, test := range refused {
		t.Run(test.name, func(t *testing.T) {
			callable, err := rt.AsCallable(test.value)
			if err == nil {
				t.Fatalf("AsCallable(%#v) answered %v", test.value, callable)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error %q does not contain %q", err, test.want)
			}
		})
	}
}

// TestAsCallableReportsWhatItCaptured covers Captures, which is the question a
// host asks before running a callable somewhere else: what came along is shared
// by every runtime running it.
func TestAsCallableReportsWhatItCaptured(t *testing.T) {
	rt := callableRuntime(t)

	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "a closure capturing nothing", value: constant(t, rt, "CLOSURE"), want: false},
		{name: "a bound method carries its receiver", value: constant(t, rt, "BOUND"), want: true},
		{name: "an __invoke carries its receiver", value: constant(t, rt, "SITE"), want: true},
		{name: "a declared function has nothing", value: "plain", want: false},
		{name: "Class::method has no receiver to share", value: "Site::shout", want: false},
		// The first-class spellings answer the same as the strings they are the
		// written form of, which is what routing closureMember through
		// newStaticMethod settles: the static one shares nothing.
		{name: "name(...) has nothing", value: constant(t, rt, "FC_FUNC"), want: false},
		{name: "$obj->method(...) carries its receiver", value: constant(t, rt, "FC_METHOD"), want: true},
		{name: "Class::method(...) has no receiver to share", value: constant(t, rt, "FC_STATIC"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			callable, err := rt.AsCallable(test.value)
			if err != nil {
				t.Fatalf("AsCallable: %v", err)
			}
			if got := callable.Captures(); got != test.want {
				t.Errorf("Captures() = %v, want %v", got, test.want)
			}
		})
	}
}
