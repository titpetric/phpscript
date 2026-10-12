package runner

import (
	"testing"
)

// TestRuntimeException covers the exception the runtime raises for a condition
// a script is expected to catch. The message is readable three ways, because a
// Go caller reads Error and a script calls getMessage; ThrowableClass is what a
// catch clause filters on, and naming RuntimeException is what puts it in reach
// of `catch (Exception $e)`, where `catch (Error $e)` does not take it.
func TestRuntimeException(t *testing.T) {
	err := NewRuntimeException("out of memory", 12)

	if got, want := err.Error(), "out of memory"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got, want := err.GetMessage(), "out of memory"; got != want {
		t.Errorf("GetMessage() = %q, want %q", got, want)
	}
	if got := err.GetCode(); got != 12 {
		t.Errorf("GetCode() = %d, want 12", got)
	}
	if got, want := err.ThrowableClass(), "RuntimeException"; got != want {
		t.Errorf("ThrowableClass() = %q, want %q", got, want)
	}

	var catchable interface{ ThrowableClass() string } = err
	if got, want := catchable.ThrowableClass(), "RuntimeException"; got != want {
		t.Errorf("through the interface a catch clause reads, ThrowableClass() = %q, want %q", got, want)
	}

	zero := NewRuntimeException("", 0)
	if zero.Error() != "" || zero.GetCode() != 0 {
		t.Errorf("an empty exception answered %q and %d", zero.Error(), zero.GetCode())
	}
}
