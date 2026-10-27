package runner

import (
	"errors"
	"fmt"
	"time"
)

// This file collects the error types a script catches. splint's grouping rule
// accepts one errors.go for a package that files them together: a caller
// reaches an error through errors.As and a script through a PHP catch clause,
// and neither navigates by filename.
//
// ThrowableClass is what decides the PHP class a catch clause matches.
// `catch (Error $e)` matches a name ending in Error and `catch (Exception $e)`
// does not, as in PHP. UndefinedConstantError names RuntimeException, and its
// own comment says why.

// ExitError reports that PHP die()/exit() interrupted script execution.
type ExitError struct {
	Code int
}

// Error returns a formatted PHP exit status.
func (e *ExitError) Error() string {
	return fmt.Sprintf("exit(%d)", e.Code)
}

// ScriptExit reports the status a script ended with, an ending and no failure.
func (e *ExitError) ScriptExit() int {
	return e.Code
}

// TimeLimitError ends a script running past set_time_limit, or one whose
// client goes away while the script heeds that.
//
// A host distinguishes the two causes through Aborted, because they call for
// different answers: a timeout is the site's problem and a disconnect is not.
type TimeLimitError struct {
	// Limit is the limit in force, zero when the client goes away first.
	Limit time.Duration

	// Aborted reports that the client closed the connection.
	Aborted bool
}

// Error says which of the two deadlines ended the script, because the client
// leaving and the clock running out read the same to everything downstream.
func (e *TimeLimitError) Error() string {
	if e.Aborted {
		return "Script ended: the client closed the connection"
	}
	// php's wording, down to the singular, for a whole number of seconds, which
	// is every limit set_time_limit can express. A handler's limit is the only
	// one that can be finer, and that one says the duration.
	if e.Limit%time.Second == 0 {
		seconds := int64(e.Limit / time.Second)
		if seconds == 1 {
			return "Maximum execution time of 1 second exceeded"
		}
		return fmt.Sprintf("Maximum execution time of %d seconds exceeded", seconds)
	}
	return fmt.Sprintf("Maximum execution time of %s exceeded", e.Limit)
}

// HostPanicError converts a panic raised by a registered Go constructor,
// function, or method into the runtime error path and PHP try/catch handles it
// like an error returned by the same host callable.
type HostPanicError struct {
	Callable string
	Value    any
}

// Error names the callable that panicked and what it panicked with.
func (e *HostPanicError) Error() string {
	return fmt.Sprintf("host panic in %s: %v", e.Callable, e.Value)
}

// ArgumentCountError reports a call that passed more arguments than the
// callable declares. PHP raises the same condition under the same name and it
// names that class: `catch (ArgumentCountError $e)` and `catch (Error $e)`
// match it and `catch (Exception $e)` does not, as in PHP.
type ArgumentCountError struct {
	Name string
	Want int
	Got  int
}

// ThrowableClass names the PHP class, implementing Throwable.
func (e *ArgumentCountError) ThrowableClass() string { return "ArgumentCountError" }

// Error is php's wording, and says "at most" because a binding spells PHP's
// optional parameters as extra ones: too few arguments is legal and zero-padded.
func (e *ArgumentCountError) Error() string {
	name := e.Name
	if name == "" {
		name = "call"
	}
	return fmt.Sprintf("%s() expects at most %s, %d given", name, plural(e.Want, "argument"), e.Got)
}

// TypeError reports an argument that will not convert to the type the
// callable's parameter declares. PHP raises TypeError for the same call and it
// names that class: `catch (TypeError $e)` and `catch (Error $e)` match it and
// `catch (Exception $e)` does not, as in PHP.
type TypeError struct {
	Name     string
	Position int
	Want     string
	Got      string
}

// ThrowableClass names the PHP class, implementing Throwable.
func (e *TypeError) ThrowableClass() string { return "TypeError" }

// Error renders the message PHP's TypeError carries for the same call.
func (e *TypeError) Error() string {
	name := e.Name
	if name == "" {
		name = "call"
	}
	// PHP also names the parameter ("Argument #3 ($limit)"). A Go binding's
	// parameter names are not recoverable through reflect and the position
	// stands alone.
	return fmt.Sprintf("%s(): Argument #%d must be of type %s, %s given", name, e.Position, e.Want, e.Got)
}

// UndefinedConstantError reports a bare name no constant, no scope value and
// no superglobal defines.
//
// PHP 8 raises Error for the same expression. This raises RuntimeException, a
// break from it: an Error in PHP is a fault in the program that a caller is
// not expected to handle, and a name this runtime does not define is an
// ordinary condition a script can catch and answer for, the same way exceeding
// memory_limit is. It also puts the condition where `catch (Exception $e)`
// already looks, which is the clause a script has written.
type UndefinedConstantError struct {
	Name string
}

// ThrowableClass names RuntimeException, which `catch (Error $e)` misses.
func (e *UndefinedConstantError) ThrowableClass() string { return "RuntimeException" }

// Error names the constant, quoted the way php quotes it.
func (e *UndefinedConstantError) Error() string {
	return fmt.Sprintf("Undefined constant %q", e.Name)
}

// IsExit reports whether PHP die()/exit() raised err.
func IsExit(err error) (*ExitError, bool) {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr, true
	}
	return nil, false
}
