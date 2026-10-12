package expr

import (
	"testing"
)

// TestBoundVar covers the three things Eval needs per variable a compiled
// expression reads: the PHP name to resolve, the slot the value lands in, and
// whether the name arrived bare.
//
// Const marks a bare name, which resolves differently: `FOO` is a constant
// where `$foo` is a variable, and the compiler cannot tell which the script
// meant without recording it.
func TestBoundVar(t *testing.T) {
	variable := BoundVar{Name: "total", Slot: 2}
	if variable.Const {
		t.Error("a $name was recorded as a bare name")
	}
	if variable.Slot != 2 {
		t.Errorf("Slot = %d, want 2", variable.Slot)
	}

	constant := BoundVar{Name: "PHP_EOL", Const: true, Slot: 0}
	if !constant.Const {
		t.Error("a bare name was recorded as a variable")
	}

	// Slot zero is a slot: the zero value and an unfilled binding read the
	// same, so the compiler fills every BoundVar it emits.
	if zero := (BoundVar{}); zero.Name != "" || zero.Slot != 0 {
		t.Errorf("the zero BoundVar is not empty: %+v", zero)
	}
}
