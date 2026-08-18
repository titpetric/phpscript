package tests

// Host bindings that panic, used to pin the panic boundary in invokeAny.
//
// A panic raised inside a registered Go function has to surface as a catchable
// PHP exception, not unwind the VM. invokeAny has two dispatch paths and only
// one of them is reflective, so both need a binding here: a signature the fast
// type switch recognises, and one that falls through to reflect.Value.Call.

// hostPanicFast has a signature invokeAny dispatches without reflection.
func hostPanicFast(v any) any {
	panic("fast path exploded")
}

// hostPanicReflect has a signature invokeAny can only call through reflection.
func hostPanicReflect(v any) (any, error) {
	panic("reflect path exploded")
}

// registerPanicBindings installs the panic bindings on either runtime, which
// share RegisterFunc but not a concrete type.
func registerPanicBindings(rt registrar) {
	rt.RegisterFunc("host_panic_fast", hostPanicFast)
	rt.RegisterFunc("host_panic_reflect", hostPanicReflect)
}
