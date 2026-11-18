package runner

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// phpThrowable is a test double for the binding-side exception value, which
// names the PHP class a script constructed it as.
type phpThrowable struct {
	class string
	msg   string
}

func (e *phpThrowable) Error() string { return e.msg }

func (e *phpThrowable) ThrowableClass() string { return e.class }

func TestMatchCatchTypeUsesTheClassName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		thrown   string
		declared string
		want     bool
	}{
		// matchCatchType compares the name and a clause takes the class it names.
		{"Exception", "Exception", true},
		{"Exception", "Throwable", true},
		{"Exception", "", true},
		{"Exception", "\\Exception", true},
		{"RuntimeException", "RuntimeException", true},
		{"RuntimeException", "runtimeexception", true},

		// Exception and Error split on the name's suffix.
		{"RuntimeException", "Exception", true},
		{"NotFound", "Exception", true},
		{"TypeError", "Exception", false},
		{"ValueError", "Exception", false},
		{"Error", "Exception", false},
		{"TypeError", "Error", true},
		{"MyError", "Error", true},
		{"RuntimeException", "Error", false},

		// A union takes the error when any alternative does.
		{"RuntimeException", "LogicException|RuntimeException", true},
		{"RuntimeException", "LogicException|TypeError", false},

		// There is no hierarchy. Every row here would pass under one and must
		// not here: see docs/design.md.
		{"InvalidArgumentException", "LogicException", false},
		{"OverflowException", "RuntimeException", false},
		{"ArgumentCountError", "TypeError", false},
		{"DivisionByZeroError", "ArithmeticError", false},
		{"RuntimeException", "LogicException", false},
	}

	for _, tt := range tests {
		err := &phpThrowable{class: tt.thrown, msg: "boom"}
		if got := matchCatchType(tt.declared, err); got != tt.want {
			t.Errorf("catch (%s) of %s: got %v, want %v", tt.declared, tt.thrown, got, tt.want)
		}
	}
}

// TestMatchCatchTypeTakesAnyClauseForANonThrowable holds the contract a host
// binding relies on: an error a Go function returned, or a panic recovered at
// the call boundary, belongs to no PHP class. The catch a script already wrote
// must reach them, whichever throwable class it names.
func TestMatchCatchTypeTakesAnyClauseForANonThrowable(t *testing.T) {
	t.Parallel()
	for _, err := range []error{
		errors.New("plain go error"),
		fmt.Errorf("wrapped: %w", errors.New("inner")),
		&HostPanicError{Callable: "Storage.Get", Value: "boom"},
	} {
		for _, declared := range []string{"Exception", "Error", "Throwable", "RuntimeException", "TypeError"} {
			if !matchCatchType(declared, err) {
				t.Errorf("catch (%s) of %v: got false, want true", declared, err)
			}
		}
	}
}

// TestMatchCatchGoRuntime covers the go runtime's one catch type.
func TestMatchCatchGoRuntime(t *testing.T) {
	t.Parallel()
	goRT := New(nil, Options{Runtime: RuntimeGo})
	phpRT := New(nil, Options{})
	for _, declared := range []string{"", "error", "\\error", "Error"} {
		for _, err := range []error{errors.New("plain"), &TypeError{Name: "strlen"}, NewRuntimeException("oom", 0)} {
			if !goRT.matchCatch(declared, err) {
				t.Errorf("go catch (%s) of %T: got false, want true", declared, err)
			}
		}
	}
	for _, declared := range []string{"Exception", "TypeError", "Throwable"} {
		if goRT.matchCatch(declared, errors.New("plain")) {
			t.Errorf("go catch (%s): got true, want false", declared)
		}
	}
	if !phpRT.matchCatch("Exception", errors.New("plain")) {
		t.Error("the php runtime stopped taking a non-throwable under any clause")
	}
	if phpRT.matchCatch("Exception", &TypeError{Name: "strlen"}) {
		t.Error("the php runtime took a TypeError under catch (Exception)")
	}
}

// TestThrowValue covers the value each runtime accepts in a throw.
func TestThrowValue(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	goRT := New(nil, Options{Runtime: RuntimeGo})
	if got := goRT.throwValue(boom); got != boom {
		t.Errorf("go throw of an error answered %v, want the error itself", got)
	}
	err := goRT.throwValue("text")
	var typeErr *TypeError
	if !errors.As(err, &typeErr) {
		t.Fatalf("go throw of a string answered %T, want *TypeError", err)
	}
	if got, want := typeErr.Error(), "throw(): Argument #1 must be of type error, string given"; got != want {
		t.Errorf("message: got %q, want %q", got, want)
	}

	phpRT := New(nil, Options{})
	if got := phpRT.throwValue("text"); got == nil || got.Error() != "uncaught exception: text" {
		t.Errorf("php throw of a string answered %v", got)
	}
	obj := model.NewObject(&model.Class{Name: "Boom"})
	if _, ok := phpRT.throwValue(obj).(*objectError); !ok {
		t.Errorf("php throw of an object did not answer *objectError")
	}
	if _, ok := goRT.throwValue(obj).(*TypeError); !ok {
		t.Errorf("go throw of an object did not answer *TypeError")
	}
}

// TestRunnerThrowablesNameTheirClass covers the throwables the runner raises
// itself. Each names a PHP class, which keeps an engine error out of
// the clauses PHP would not route it to.
func TestRunnerThrowablesNameTheirClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err      error
		declared string
		want     bool
	}{
		{NewRuntimeException("out of memory", 0), "RuntimeException", true},
		{NewRuntimeException("out of memory", 0), "Exception", true},
		{NewRuntimeException("out of memory", 0), "Error", false},
		{&ArithmeticError{Message: "negative shift"}, "ArithmeticError", true},
		{&ArithmeticError{Message: "negative shift"}, "Error", true},
		{&ArithmeticError{Message: "negative shift"}, "Exception", false},
		{&TypeError{Name: "explode"}, "TypeError", true},
		{&TypeError{Name: "explode"}, "Error", true},
		{&TypeError{Name: "explode"}, "Exception", false},
		{&ArgumentCountError{Name: "strlen"}, "ArgumentCountError", true},
		{&ArgumentCountError{Name: "strlen"}, "Error", true},
		{&ArgumentCountError{Name: "strlen"}, "Exception", false},
	}

	for _, tt := range tests {
		if got := matchCatchType(tt.declared, tt.err); got != tt.want {
			t.Errorf("catch (%s) of %T: got %v, want %v", tt.declared, tt.err, got, tt.want)
		}
	}
}

// TestNoInheritanceAtRuntime is the executable form of a design decision:
// phpscript has no OOP, and `extends` confers nothing. docs/design.md refuses
// it; read that before changing this test.
func TestNoInheritanceAtRuntime(t *testing.T) {
	t.Parallel()
	// model.Class may record what a declaration listed and Implements stays:
	// a list of names a class checks against, and no member arrives through
	// it. A parent is different, and forbidden: a class that has one would
	// start acquiring members it did not declare.
	for _, name := range []string{"Parent", "Extends", "Parents", "Super", "Base"} {
		if _, found := reflect.TypeOf(model.Class{}).FieldByName(name); found {
			t.Fatalf("model.Class has a %s field: phpscript has no inheritance, `extends` is recorded on model.ClassDecl for the formatter only. See docs/design.md.", name)
		}
	}

	// A class declaring `extends RuntimeException` counts as that class alone,
	// to a catch clause and to instanceof alike.
	thrown := newObjectError(model.NewObject(&model.Class{Name: "NotFound"}))
	if matchCatchType("RuntimeException", thrown) {
		t.Error("catch (RuntimeException) took a NotFound: `extends` must confer nothing")
	}
	if !matchCatchType("NotFound", thrown) {
		t.Error("catch (NotFound) did not take a NotFound")
	}

	obj := model.NewObject(&model.Class{Name: "NotFound"})
	if phpInstanceOf(obj, "RuntimeException") {
		t.Error("instanceof RuntimeException was true for a NotFound: `extends` must confer nothing")
	}
	if !phpInstanceOf(obj, "notfound") {
		t.Error("instanceof is class-name equality and is case-insensitive")
	}
}
