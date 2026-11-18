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

// boundScope snapshots the running frame as an interpreter scope before the callee runs.
func (h *flatHost) boundScope() *Scope {
	scope := h.runtime.newScope()
	if h.frame != nil {
		for name, value := range h.frame.Snapshot() {
			scope.Set(name, value)
		}
	}
	return scope
}

// pullScope writes a materialised scope back into the frame, dropping the magic constants.
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
	// for the asking methods alone, which is no common data-access shape.
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

// Call resolves a function by name, materialising a scope only for context-taking bindings.
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

// callResolved invokes a table hit through a scope when its invoker takes context.
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

// InvokeValue calls a callable held in a value through the interpreter's resolution.
func (h *flatHost) InvokeValue(args []any, callee any) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperInvoke(&scopeRef{scope: scope})(callee, args...)
	h.pullScope(scope)
	return result, err
}

// CallableValue takes a first-class callable of a name or a value.
func (h *flatHost) CallableValue(target any, fallback string) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperCallable(&scopeRef{scope: scope})(target, fallback)
	h.pullScope(scope)
	return result, err
}

// CallableMember takes a first-class callable of a method, forwarding the frame's this.
func (h *flatHost) CallableMember(target any, method any) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperCallableMember(&scopeRef{scope: scope})(target, method)
	h.pullScope(scope)
	return result, err
}

// CallStatic dispatches Class::method through host statics and declared PHP methods.
func (h *flatHost) CallStatic(class string, method any, args []any) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperStaticCall(&scopeRef{scope: scope})(class, method, args...)
	h.pullScope(scope)
	return result, err
}

// GetStaticProp reads Class::$name, autoloading the class first.
func (h *flatHost) GetStaticProp(class, name string) (any, error) {
	scope := h.boundScope()
	result, err := h.runtime.helperStaticProp(&scopeRef{scope: scope})(class, name)
	h.pullScope(scope)
	return result, err
}

// SetStaticProp writes Class::$name, applying op against the current value.
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
	// A compiled closure arrives as a func value and reads its frame in a
	// nested run. Only a context-taking callable needs the scope materialised.
	if !wantsContext(reflect.TypeOf(callable)) {
		_, err := h.runtime.invokeWithScopeContext(callable, nil, nil)
		return err
	}
	scope := h.boundScope()
	_, err := h.runtime.invokeWithScopeContext(callable, nil, scope)
	h.pullScope(scope)
	return err
}
