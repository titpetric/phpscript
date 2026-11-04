package runner

import (
	"testing"
)

// TestOperandError covers the class a script catches an operation between two
// widths under. php words the refusal "Unsupported operand types" and raises
// TypeError for it, and the name a catch clause reads is that class and not
// the Go type's own.
func TestOperandError(t *testing.T) {
	t.Parallel()
	err := &OperandError{Message: "Unsupported operand types: int8 + int16"}
	if got, want := err.Error(), "Unsupported operand types: int8 + int16"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	class, ok := throwableClassOf(err)
	if !ok {
		t.Fatal("throwableClassOf() = false on an OperandError")
	}
	if want := "TypeError"; class != want {
		t.Errorf("ThrowableClass() = %q, want %q", class, want)
	}
	if got := err.ThrowableClass(); got != class {
		t.Errorf("ThrowableClass() = %q, read off the value, against %q through throwableClassOf", got, class)
	}
}
