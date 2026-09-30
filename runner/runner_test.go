package runner

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// phpThrowable stands in for the binding-side exception value, which knows the
// PHP class a script constructed it as.
type phpThrowable struct {
	class string
	msg   string
}

func (e *phpThrowable) Error() string { return e.msg }

func (e *phpThrowable) ThrowableClass() string { return e.class }

func TestMatchCatchTypeUsesTheClassName(t *testing.T) {
	tests := []struct {
		thrown   string
		declared string
		want     bool
	}{
		// The name is compared, so a clause takes the class it names.
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

// TestMatchCatchTypeTakesAnyClauseForANonThrowable is the contract a host
// binding is written against: an error a Go function returned, and a panic
// recovered at the call boundary, belong to no PHP class, so the catch a script
// already wrote has to reach them whichever throwable class it names.
func TestMatchCatchTypeTakesAnyClauseForANonThrowable(t *testing.T) {
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

// TestRunnerThrowablesNameTheirClass covers the throwables the runner raises
// itself. Each names a PHP class, which is what keeps an engine error out of
// the clauses PHP would not offer it to.
func TestRunnerThrowablesNameTheirClass(t *testing.T) {
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
// phpscript has no OOP, and `extends` confers nothing. It was implemented once
// and rejected. If this test fails, read docs/design.md before changing it.
func TestNoInheritanceAtRuntime(t *testing.T) {
	// model.Class may record what a declaration listed, which is why
	// Implements is allowed: it is a list of names a class was checked
	// against, and no member arrives through it. A parent is different, and
	// forbidden: a class that has one would start acquiring members it did not
	// declare.
	for _, name := range []string{"Parent", "Extends", "Parents", "Super", "Base"} {
		if _, found := reflect.TypeOf(model.Class{}).FieldByName(name); found {
			t.Fatalf("model.Class has a %s field: phpscript has no inheritance, `extends` is recorded on model.ClassDecl for the formatter only. See docs/design.md.", name)
		}
	}

	// A class declaring `extends RuntimeException` is its own class and nothing
	// else, to a catch clause and to instanceof alike.
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
