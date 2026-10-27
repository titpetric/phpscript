package runner

import (
	"reflect"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// This file is the call half of the bytecode host: every method that
// materialises the running frame as an interpreter scope to re-enter the
// interpreter's dispatch. The value operations the host answers in place stay
// in flatstack.go.

// boundScope materialises the running frame as an interpreter scope, for the
// callees that read or write caller locals. It snapshots the frame before the
// callee body runs, the ordering the by-reference marks rely on.
func (h *flatHost) boundScope() *Scope {
	scope := h.runtime.newScope()
	if h.frame != nil {
		for name, value := range h.frame.Snapshot() {
			scope.Set(name, value)
		}
	}
	return scope
}

// pullScope writes a materialised scope's variables back into the frame.
// boundScope builds that scope for one call and nothing keeps it and this
// deletes the magic constants in place and filters nothing into another map.
func (h *flatHost) pullScope(scope *Scope) {
	if h.frame == nil {
		return
	}
	delete(scope.vars, "__FILE__")
	delete(scope.vars, "__DIR__")
	h.frame.WriteBack(scope.vars)
}

func (h *flatHost) Construct(class string, args []any) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperNew(&scopeRef{scope: scope})(strings.TrimPrefix(class, "\\"), args...)
	h.pullScope(scope)
	return result, err
}

func (h *flatHost) CallMethod(receiver any, method string, args []any) (any, error) {
	// A method on a PHP-declared object dispatches through the interpreter
	// and sees the frame. A Go receiver's method sees the frame only through
	// a context parameter and callGoMethod's scopeFor materialises the scope
	// for the methods that ask and no others, which is no common data-access
	// shape.
	if obj, ok := receiver.(*model.Object); ok && obj.Class != nil {
		scope := h.boundScope()
		result, err := h.runtime.helperCall(&scopeRef{scope: scope})(receiver, method, args...)
		h.pullScope(scope)
		return result, err
	}
	var scope *Scope
	result, err := h.runtime.callGoMethod(receiver, method, args, func() *Scope {
		scope = h.boundScope()
		return scope
	})
	if scope != nil {
		h.pullScope(scope)
	}
	return result, err
}

// Call resolves a function the way helperFunc does, but a binding whose
// signature declares no context never sees the scope and this builds it no
// snapshot, no Scope and no write-back. That is the interpreter's own
// contract: installFunc calls the same bindings with no per-call scope.
func (h *flatHost) Call(fnName, fallback string, args []any) (any, error) {
	if entry, ok := h.runtime.lookupEntry(fnName); ok {
		return h.callResolved(entry, fnName, args)
	}
	if fallback != "" {
		if entry, ok := h.runtime.lookupEntry(fallback); ok {
			return h.callResolved(entry, fallback, args)
		}
	}
	// Frame-aware builtins (func_get_args) and the undefined-function error
	// live behind helperFunc; both need the scope.
	scope := h.boundScope()
	result, err := h.runtime.helperFunc(&scopeRef{scope: scope})(fnName, fallback, args...)
	h.pullScope(scope)
	return result, err
}

// callResolved invokes a function-table hit: lean when the entry's invoker
// declares no context, through a materialised scope when it does. The
// panic boundary, the argument-count check and the memory burst guard all
// sit in invokeEntry either way.
func (h *flatHost) callResolved(entry *funcEntry, name string, args []any) (any, error) {
	if !entry.invoker().wantsCtx {
		result, err := h.runtime.invokeEntry(entry, args, nil)
		return result, nameCallError(err, name)
	}
	scope := h.boundScope()
	result, err := h.runtime.invokeEntry(entry, args, scope)
	h.pullScope(scope)
	return result, nameCallError(err, name)
}

// InvokeValue calls a callable held in a value, `$fn(...)`, through the same
// resolution call_user_func uses and every callable spelling works.
func (h *flatHost) InvokeValue(args []any, callee any) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperInvoke(&scopeRef{scope: scope})(callee, args...)
	h.pullScope(scope)
	return result, err
}

// CallableValue takes `greet(...)` and `$fn(...)`, php 8.1's first-class
// callable syntax over a name or a value, through the interpreter's resolution.
//
// A name the flat compiler declared sits in the function table too - runFlat
// hoists before it runs - so the Closure resolves here and the body it names
// runs on the interpreter when something calls the value, the way a callable
// held in a value already does.
func (h *flatHost) CallableValue(target any, fallback string) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperCallable(&scopeRef{scope: scope})(target, fallback)
	h.pullScope(scope)
	return result, err
}

// CallableMember takes `$obj->method(...)` and `Class::method(...)`. The frame
// snapshot is what carries `this` across, as it does for a static call.
func (h *flatHost) CallableMember(target any, method any) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperCallableMember(&scopeRef{scope: scope})(target, method)
	h.pullScope(scope)
	return result, err
}

// CallStatic dispatches `Class::method(args...)` through the interpreter's
// resolution: host statics, then a declared PHP method with the current
// `$this` forwarded for the `self::` instance-call form. The frame snapshot
// is what carries `this` across.
func (h *flatHost) CallStatic(class string, method any, args []any) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperStaticCall(&scopeRef{scope: scope})(class, method, args...)
	h.pullScope(scope)
	return result, err
}

// GetStaticProp reads `Class::$name`, autoloading the class the way the
// interpreter would.
func (h *flatHost) GetStaticProp(class, name string) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperStaticProp(&scopeRef{scope: scope})(class, name)
	h.pullScope(scope)
	return result, err
}

// SetStaticProp writes `Class::$name`, applying a compound operator against
// the current value under the same rules a property assignment uses.
func (h *flatHost) SetStaticProp(class, name string, value any, op string) error {
	scope := h.boundScope()
	defer h.pullScope(scope)
	bag, err := h.runtime.staticStorage(class, scope)
	if err != nil {
		return err
	}
	next, err := applyAssignOp(op, bag[name], value)
	if err != nil {
		return err
	}
	bag[name] = next
	return nil
}

func (h *flatHost) InvokeCallable(callable any) error {
	if callable == nil {
		return nil
	}
	// A compiled closure arrives as func(...any) (any, error) and reads its
	// frame through its own nested run and only a context-taking callable
	// needs the scope materialised.
	if !wantsContext(reflect.TypeOf(callable)) {
		_, err := h.runtime.invokeWithScopeContext(callable, nil, nil)
		return err
	}
	scope := h.boundScope()
	_, err := h.runtime.invokeWithScopeContext(callable, nil, scope)
	h.pullScope(scope)
	return err
}
