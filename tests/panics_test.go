package tests

import (
	"testing"
)

// TestRegisterPanicBindings covers the two signatures that pin the panic
// boundary. invokeAny calls a func(any) any from its type switch and reaches
// a func(any) (any, error) only through reflect.Value.Call and a binding that
// panics has to exist for each: one registration of the wrong shape would
// leave the reflective path unchecked.
func TestRegisterPanicBindings(t *testing.T) {
	funcs := map[string]any{}
	registerPanicBindings(&collector{funcs: funcs})

	fast, ok := funcs["host_panic_fast"].(func(any) any)
	if !ok {
		t.Fatalf("host_panic_fast is registered as %T, want func(any) any", funcs["host_panic_fast"])
	}
	reflective, ok := funcs["host_panic_reflect"].(func(any) (any, error))
	if !ok {
		t.Fatalf("host_panic_reflect is registered as %T, want func(any) (any, error)", funcs["host_panic_reflect"])
	}

	if got := recovered(func() { fast(nil) }); got != "fast path exploded" {
		t.Errorf("host_panic_fast panicked with %v, want \"fast path exploded\"", got)
	}
	if got := recovered(func() { _, _ = reflective(nil) }); got != "reflect path exploded" {
		t.Errorf("host_panic_reflect panicked with %v, want \"reflect path exploded\"", got)
	}
}

// recovered answers the value fn panicked with, or nil for a call that
// returned.
func recovered(fn func()) (value any) {
	defer func() { value = recover() }()
	fn()
	return nil
}
