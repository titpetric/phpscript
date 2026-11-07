package runner

import (
	"github.com/titpetric/phpscript/model"
)

// This file resolves where an index assignment lands: the container the write
// goes into, created where php creates one, and the store itself. The writes
// that reach a native Go value go through assign_go.go.

// container evaluates the base of an index assignment, creating the array PHP
// would create when nothing is there yet. `$rows[$first][$second] = $v` against
// an unset $rows makes both levels, and each level runs its own key once: the
// created array goes back through the container and the key already in hand.
//
// Only an assignable base is auto-vivified. A function result or a literal has
// nowhere to write the array back to and container returns it as it stands and
// the caller reports the assignment as unsupported.
func (rt *Runtime) container(base model.Expr, scope *Scope) (any, error) {
	switch node := model.UnwrapParenthesized(base).(type) {
	case *model.Index:
		if node.Index != nil {
			return rt.indexContainer(node, scope)
		}
	case *model.PropAccess:
		return rt.propContainer(node, scope)
	}
	value, err := rt.Eval(base, scope)
	if err != nil {
		return nil, err
	}
	if value != nil || !assignable(base) {
		return value, nil
	}
	created := model.NewArray()
	if err := rt.assignTo(base, created, scope); err != nil {
		return nil, err
	}
	return created, nil
}

func (rt *Runtime) indexContainer(node *model.Index, scope *Scope) (any, error) {
	parent, err := rt.container(node.Base, scope)
	if err != nil {
		return nil, err
	}
	key, err := rt.Eval(node.Index, scope)
	if err != nil {
		return nil, err
	}
	if value := helperIndex(parent, key); value != nil {
		return value, nil
	}
	created := model.NewArray()
	if err := storeIndex(parent, key, created); err != nil {
		return nil, err
	}
	return created, nil
}

func (rt *Runtime) propContainer(node *model.PropAccess, scope *Scope) (any, error) {
	receiver, err := rt.Eval(node.Base, scope)
	if err != nil {
		return nil, err
	}
	name, err := rt.propName(node, scope)
	if err != nil {
		return nil, err
	}
	if value := rt.helperGet(&scopeRef{scope: scope})(receiver, name); value != nil {
		return value, nil
	}
	created := model.NewArray()
	if err := storeProp(receiver, name, created); err != nil {
		return nil, err
	}
	return created, nil
}

// storeIndex writes val at key of base, as `$base[$key] = $val` does.
func storeIndex(base, key, val any) error {
	switch b := base.(type) {
	case *model.Array:
		b.Set(normalizeKey(key), val)
		return nil
	case model.Keyed:
		b.Write(keyString(key), val)
		return nil
	}
	return assignGoIndex(base, key, func(any) (any, error) { return val, nil })
}

// storeProp writes val to the property name of base, as `$base->name = $val`.
func storeProp(base any, name string, val any) error {
	if obj, ok := base.(*model.Object); ok {
		obj.SetProp(name, val)
		return nil
	}
	return assignGoField(base, name, func(any) (any, error) { return val, nil })
}
