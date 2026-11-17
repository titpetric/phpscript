package runner

import (
	"fmt"
	"reflect"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// This file is the go runtime's pointer values: `&x` answers a Pointer over
// the storage x names, `*$p` reads it, and an assignment through one writes
// it. A binding's native `*T` takes the same two operations by reflection.

// Pointer is the value `&x` answers: the address a `*$p` reads and writes.
type Pointer struct {
	get func() (any, error)
	set func(any) error
}

// Get answers the value the pointer addresses.
func (p *Pointer) Get() (any, error) { return p.get() }

// Set writes v to the storage the pointer addresses.
func (p *Pointer) Set(v any) error { return p.set(v) }

// evalAddrOf answers `&x`: a pointer to the variable or element x names.
func (rt *Runtime) evalAddrOf(n *model.AddrOf, scope *Scope) (any, error) {
	switch tgt := modelutil.UnwrapParenthesized(n.X).(type) {
	case *model.Var:
		name := tgt.Name
		return &Pointer{
			get: func() (any, error) { return rt.resolveVar(name, varIdent(name), scope) },
			set: func(v any) error { rt.setVar(scope, name, v); return nil },
		}, nil
	case *model.Index:
		if tgt.Index == nil {
			return nil, fmt.Errorf("& takes the address of an element and [] names none")
		}
		base, err := rt.container(tgt.Base, scope)
		if err != nil {
			return nil, err
		}
		key, err := rt.Eval(tgt.Index, scope)
		if err != nil {
			return nil, err
		}
		return &Pointer{
			get: func() (any, error) { return helperIndex(base, key), nil },
			set: func(v any) error {
				if arr, ok := base.(*model.Array); ok {
					arr.Set(normalizeKey(key), v)
					return nil
				}
				return assignGoIndex(base, key, func(any) (any, error) { return v, nil })
			},
		}, nil
	}
	return nil, fmt.Errorf("& takes the address of a variable, not a %T", n.X)
}

// derefValue answers `*$p` for a script pointer or a binding's native one.
func derefValue(v any) (any, error) {
	if p, ok := v.(*Pointer); ok {
		return p.Get()
	}
	rv := reflect.ValueOf(v)
	if rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, fmt.Errorf("dereference of a nil %T", v)
		}
		if !rv.Elem().CanInterface() {
			return nil, fmt.Errorf("dereference of a %T reads a private field", v)
		}
		return rv.Elem().Interface(), nil
	}
	return nil, fmt.Errorf("cannot dereference a %T", v)
}

// assignDeref stores the value `*$p = v` computes through the pointer.
func (rt *Runtime) assignDeref(tgt *model.Deref, op string, value model.Expr, scope *Scope) (any, error) {
	ptr, err := rt.Eval(tgt.X, scope)
	if err != nil {
		return nil, err
	}
	rhs, err := rt.Eval(value, scope)
	if err != nil {
		return nil, err
	}
	var stored any
	err = writePointer(ptr, func(current any) (any, error) {
		next, err := applyAssignOp(op, current, rhs)
		stored = next
		return next, err
	})
	return stored, err
}

// writePointer applies next to the value the pointer addresses and stores it.
func writePointer(v any, next func(current any) (any, error)) error {
	if p, ok := v.(*Pointer); ok {
		current, err := p.Get()
		if err != nil {
			return err
		}
		updated, err := next(current)
		if err != nil {
			return err
		}
		return p.Set(updated)
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("cannot assign through a %T", v)
	}
	elem := rv.Elem()
	if !elem.CanSet() || !elem.CanInterface() {
		return fmt.Errorf("cannot assign through a %T", v)
	}
	updated, err := next(elem.Interface())
	if err != nil {
		return err
	}
	if updated == nil {
		elem.Set(reflect.Zero(elem.Type()))
		return nil
	}
	out := reflect.ValueOf(updated)
	if !out.Type().AssignableTo(elem.Type()) {
		if !out.Type().ConvertibleTo(elem.Type()) {
			return fmt.Errorf("cannot assign a %T through a %T", updated, v)
		}
		out = out.Convert(elem.Type())
	}
	elem.Set(out)
	return nil
}
