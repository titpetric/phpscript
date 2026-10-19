package runner

import (
	"testing"
)

// TestArithmeticError covers the class name a catch clause takes a shift by a
// negative number under. There is no class hierarchy here, so matchCatchType
// reads the name off ThrowableClass and matches "Error" on its suffix: the
// name has to end in Error for `catch (Error $e)` to take it.
func TestArithmeticError(t *testing.T) {
	t.Parallel()
	err := &ArithmeticError{Message: "Bit shift by negative number"}
	if got, want := err.Error(), "Bit shift by negative number"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	class, ok := throwableClassOf(err)
	if !ok {
		t.Fatal("throwableClassOf() = false on an ArithmeticError")
	}
	if want := "ArithmeticError"; class != want {
		t.Errorf("ThrowableClass() = %q, want %q", class, want)
	}
}

// TestDivisionByZeroError covers the class intdiv() and `%` raise on a zero
// divisor. PHP 8 made it a class of its own under Error, so a script catching
// DivisionByZeroError takes it and one catching Exception does not.
func TestDivisionByZeroError(t *testing.T) {
	t.Parallel()
	err := &DivisionByZeroError{Message: "Modulo by zero"}
	if got, want := err.Error(), "Modulo by zero"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	class, ok := throwableClassOf(err)
	if !ok {
		t.Fatal("throwableClassOf() = false on a DivisionByZeroError")
	}
	if want := "DivisionByZeroError"; class != want {
		t.Errorf("ThrowableClass() = %q, want %q", class, want)
	}
	if got := err.ThrowableClass(); got != class {
		t.Errorf("ThrowableClass() = %q, read off the value, against %q through throwableClassOf", got, class)
	}
	if class == (&ArithmeticError{}).ThrowableClass() {
		t.Error("the two arithmetic classes report the same name")
	}
}
