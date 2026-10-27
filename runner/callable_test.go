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
// callable contract: AsCallable answers a value another runtime can run and it
// needs a declaration to run and refuses anything that is not one.
//
// The two array spellings are the documented refusal (docs/README.md): they stay
// callable everywhere else. Class::method is not one of them, and the
// "no PHP function of that name" refusal describes no method a class declares.
func TestAsCallableTakesEverySpellingButAnArray(t *testing.T) {
	t.Parallel()
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
		// The first-class spellings arrive as a *Callable already and the router
		// takes the form php itself writes, and not only the string for it.
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

// TestCallable_Invoke covers the cross-runtime call a request answered on a
// worker of its own makes: the value crosses and the execution does not and
// each spelling runs on the runtime passed in.
//
// A closure, a bound method and Class::method carry their declaration and run
// on a runtime that never loaded the source. A name crosses as a name and the
// runtime running it resolves it against its own function table, and one
// declaring nothing cannot answer for it. That is the half that lets a host
// register a binding under the name a script named.
func TestCallable_Invoke(t *testing.T) {
	t.Parallel()
	origin := callableRuntime(t)
	bare := runner.New(io.Discard, runner.Options{})
	stdlib.Register(bare)

	carried := []struct {
		name  string
		value any
		want  string
	}{
		{name: "closure", value: constant(t, origin, "CLOSURE"), want: "closure b"},
		{name: "bound method", value: constant(t, origin, "BOUND"), want: "hei b"},
		{name: "Class::method", value: "Site::shout", want: "HELLO b"},
		{name: "an object with __invoke", value: constant(t, origin, "SITE"), want: "invoked b"},
	}
	for _, test := range carried {
		t.Run(test.name, func(t *testing.T) {
			callable, err := origin.AsCallable(test.value)
			if err != nil {
				t.Fatalf("AsCallable: %v", err)
			}
			result, err := callable.Invoke(bare, "b")
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if got, _ := result.(string); got != test.want {
				t.Errorf("Invoke() = %v, want %q", result, test.want)
			}
		})
	}

	named, err := origin.AsCallable("plain")
	if err != nil {
		t.Fatal(err)
	}
	if result, err := named.Invoke(bare, "b"); err == nil {
		t.Errorf("a name resolved to %v on a runtime that declares nothing", result)
	}
	result, err := named.Invoke(callableRuntime(t), "b")
	if err != nil {
		t.Fatalf("Invoke on a runtime declaring plain(): %v", err)
	}
	if got, _ := result.(string); got != "plain b" {
		t.Errorf("Invoke() = %v, want %q", result, "plain b")
	}

	// Call is the same execution on the runtime that built the value and the
	// two answers agree where the runtime is the same one.
	closure, err := origin.AsCallable(constant(t, origin, "CLOSURE"))
	if err != nil {
		t.Fatal(err)
	}
	direct, err := closure.Call("b")
	if err != nil {
		t.Fatal(err)
	}
	crossed, err := closure.Invoke(origin, "b")
	if err != nil {
		t.Fatal(err)
	}
	if direct != crossed {
		t.Errorf("Call() = %v and Invoke(origin) = %v, want the same", direct, crossed)
	}
}

// TestAsCallableReportsWhatItCaptured covers Captures, which is the question a
// host asks before running a callable somewhere else: every runtime running it
// shares what came along.
func TestAsCallableReportsWhatItCaptured(t *testing.T) {
	t.Parallel()
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
		// written form of and routing closureMember through
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
