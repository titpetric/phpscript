package runner

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// TestExitError covers the wording and the status die()/exit() ends a script
// with. ScriptExit is the interface flatstack/engine recognises an exit
// through, because runner imports engine and engine cannot import runner back.
func TestExitError(t *testing.T) {
	t.Parallel()
	err := &ExitError{Code: 3}
	if got, want := err.Error(), "exit(3)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got := err.ScriptExit(); got != 3 {
		t.Errorf("ScriptExit() = %d, want 3", got)
	}

	var exit interface{ ScriptExit() int } = err
	if got := exit.ScriptExit(); got != 3 {
		t.Errorf("through the interface, ScriptExit() = %d, want 3", got)
	}
}

// TestIsExit covers recognising an exit through a wrapper: a host tells a
// script that ended from one that failed by that check.
func TestIsExit(t *testing.T) {
	t.Parallel()
	exit, ok := IsExit(fmt.Errorf("while running: %w", &ExitError{Code: 7}))
	if !ok {
		t.Fatal("IsExit() = false on a wrapped ExitError")
	}
	if exit.Code != 7 {
		t.Errorf("the unwrapped error carries code %d, want 7", exit.Code)
	}

	if _, ok := IsExit(errors.New("ordinary failure")); ok {
		t.Error("IsExit() = true on an error that is not an exit")
	}
	if _, ok := IsExit(nil); ok {
		t.Error("IsExit(nil) = true")
	}
}

// TestTimeLimitError covers php's own wording for a script that ran out of
// time, down to the singular, and the separate message for a client that left.
// A host reads Aborted to tell them apart: a timeout is the site's problem and
// a disconnect is not.
func TestTimeLimitError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  TimeLimitError
		want string
	}{
		{TimeLimitError{Limit: time.Second}, "Maximum execution time of 1 second exceeded"},
		{TimeLimitError{Limit: 30 * time.Second}, "Maximum execution time of 30 seconds exceeded"},
		{TimeLimitError{Limit: 1500 * time.Millisecond}, "Maximum execution time of 1.5s exceeded"},
		{TimeLimitError{Aborted: true}, "Script ended: the client closed the connection"},
	} {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
	}
}

// TestArgumentCountError covers php's wording for a call with too many
// arguments. It says "at most" because a binding spells PHP's optional
// parameters as extra ones, so too few arguments is legal and zero-padded.
func TestArgumentCountError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  ArgumentCountError
		want string
	}{
		{ArgumentCountError{Name: "substr", Want: 3, Got: 4}, "substr() expects at most 3 arguments, 4 given"},
		{ArgumentCountError{Name: "strlen", Want: 1, Got: 2}, "strlen() expects at most 1 argument, 2 given"},
		{ArgumentCountError{Want: 1, Got: 2}, "call() expects at most 1 argument, 2 given"},
	} {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
	}

	if got, want := (&ArgumentCountError{}).ThrowableClass(), "ArgumentCountError"; got != want {
		t.Errorf("ThrowableClass() = %q, want %q", got, want)
	}
}

// TestTypeError covers the message PHP's TypeError carries. The message names
// the parameter by position and not by name, because a Go binding's parameter
// names are not recoverable through reflect.
func TestTypeError(t *testing.T) {
	t.Parallel()
	err := &TypeError{Name: "str_repeat", Position: 2, Want: "int", Got: "string"}
	want := "str_repeat(): Argument #2 must be of type int, string given"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got := (&TypeError{Position: 1, Want: "int", Got: "array"}).Error(); got == "" {
		t.Error("Error() on a nameless TypeError answered nothing")
	}
	if got, want := err.ThrowableClass(), "TypeError"; got != want {
		t.Errorf("ThrowableClass() = %q, want %q", got, want)
	}
}

// TestUndefinedConstantError covers the one error here that names a class PHP
// does not: RuntimeException, so `catch (Exception $e)` takes it. A name this
// runtime does not define is a condition a script can answer for, where PHP 8
// raises Error and expects nobody to handle it.
func TestUndefinedConstantError(t *testing.T) {
	t.Parallel()
	err := &UndefinedConstantError{Name: "MY_FLAG"}
	if got, want := err.Error(), `Undefined constant "MY_FLAG"`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got, want := err.ThrowableClass(), "RuntimeException"; got != want {
		t.Errorf("ThrowableClass() = %q, want %q", got, want)
	}
}

// TestHostPanicError covers the error a panic in a registered Go callable
// reaches a script as, so a PHP try/catch handles it the way it handles an
// error the same callable returned.
func TestHostPanicError(t *testing.T) {
	t.Parallel()
	err := &HostPanicError{Callable: "Storage::open", Value: "nil map"}
	if got, want := err.Error(), "host panic in Storage::open: nil map"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
