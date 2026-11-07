package runner

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// This file mutates storage a script names: assignment and unset over every
// lvalue shape, the read an increment makes first, and the compound operators
// both resolve through. The container a write lands in is assign_container.go,
// and the writes that reach a binding's native Go value assign_go.go.

// execAssign mutates a variable, property or array element. expr-lang cannot do
// this and it lives entirely here.
func (rt *Runtime) execAssign(n *model.Assign, scope *Scope) error {
	_, err := rt.assignValue(n.Target, n.Op, n.Value, scope)
	return err
}

// assignValue stores value in target and answers what the target holds after
// the store, the value php reads from an assignment used as one. A chain
// stores outside in and each link answers its own write.
func (rt *Runtime) assignValue(target model.Expr, op string, value model.Expr, scope *Scope) (any, error) {
	// `$a ??= b` reads the target once and stores only while it answers null:
	// storing the value already there would write a string offset php leaves
	// untouched. See coalesceAssign.
	if co, ok := value.(*model.Coalesce); ok && co.Assign {
		return rt.coalesceAssign(target, co.Right, scope)
	}
	// php resolves the container, the key and the receiver of the target before
	// the value, the order `$m[$i++] = $i` reads. A target that names storage
	// runs nothing and takes the value first.
	switch tgt := model.UnwrapParenthesized(target).(type) {
	case *model.Index:
		return rt.assignIndex(tgt, op, value, scope)
	case *model.PropAccess:
		return rt.assignProp(tgt, op, value, scope)
	}
	rhs, err := rt.Eval(value, scope)
	if err != nil {
		return nil, err
	}
	bindConstructorID(target, value, op, rhs)

	switch tgt := model.UnwrapParenthesized(target).(type) {
	case *model.Var:
		cur, _ := scope.Get(tgt.Name)
		next, err := applyAssignOp(op, cur, rhs)
		if err != nil {
			return nil, err
		}
		rt.setVar(scope, tgt.Name, next)
		return next, nil

	case *model.ListExpr:
		// list($a, $b) = $arr destructures by position (integer keys). The
		// loop below reads the source through helperIndex and a binding
		// returning a native []string destructures like an *Array.
		if !model.IsCollection(rhs) {
			return nil, fmt.Errorf("assign: list() target requires an array")
		}
		for i, el := range tgt.Elems {
			if el == nil {
				continue
			}
			if err := rt.assignTo(el, helperIndex(rhs, int64(i)), scope); err != nil {
				return nil, err
			}
		}
		return rhs, nil

	case *model.StaticProp:
		bag, err := rt.staticStorage(tgt.Class, scope)
		if err != nil {
			return nil, err
		}
		next, err := applyAssignOp(op, bag[tgt.Name], rhs)
		if err != nil {
			return nil, err
		}
		bag[tgt.Name] = next
		return next, nil

	default:
		return nil, fmt.Errorf("assign: unsupported target %T", target)
	}
}

// assignIndex stores value at one element, resolving the container and the key
// ahead of it. It answers what the element holds after the store.
func (rt *Runtime) assignIndex(tgt *model.Index, op string, value model.Expr, scope *Scope) (any, error) {
	base, err := rt.container(tgt.Base, scope)
	if err != nil {
		return nil, err
	}
	appendValue := op == "[]=" || tgt.Index == nil
	var key any
	if !appendValue {
		if key, err = rt.Eval(tgt.Index, scope); err != nil {
			return nil, err
		}
	}
	rhs, err := rt.Eval(value, scope)
	if err != nil {
		return nil, err
	}
	arr, ok := base.(*model.Array)
	if !ok {
		// Native Go collections are writable where Go itself allows it: a
		// map takes a new or replacement entry, a slice an overwrite of an
		// element it holds. Only `$a[] =` needs an *Array, since a slice
		// cannot grow through the interface holding it.
		if appendValue {
			return nil, fmt.Errorf("assign: cannot append to %T; a binding whose result is appended to must return *model.Array", base)
		}
		var stored any
		err = assignGoIndex(base, key, func(current any) (any, error) {
			next, err := applyAssignOp(op, current, rhs)
			stored = next
			return next, err
		})
		return stored, err
	}
	if appendValue {
		arr.Append(rhs)
		return rhs, nil
	}
	k := normalizeKey(key)
	cur, _ := arr.Get(k)
	next, err := applyAssignOp(op, cur, rhs)
	if err != nil {
		return nil, err
	}
	arr.Set(k, next)
	return next, nil
}

// assignProp stores value in one property, resolving the receiver and the name
// ahead of it. It answers what the property holds after the store.
func (rt *Runtime) assignProp(tgt *model.PropAccess, op string, value model.Expr, scope *Scope) (any, error) {
	base, err := rt.Eval(tgt.Base, scope)
	if err != nil {
		return nil, err
	}
	name, err := rt.propName(tgt, scope)
	if err != nil {
		return nil, err
	}
	rhs, err := rt.Eval(value, scope)
	if err != nil {
		return nil, err
	}
	if obj, ok := base.(*model.Object); ok {
		next, err := applyAssignOp(op, obj.Props[name], rhs)
		if err != nil {
			return nil, err
		}
		obj.SetProp(name, next)
		return next, nil
	}
	var stored any
	err = assignGoField(base, name, func(current any) (any, error) {
		next, err := applyAssignOp(op, current, rhs)
		stored = next
		return next, err
	})
	return stored, err
}

func bindConstructorID(target, value model.Expr, op string, result any) {
	if op != "" && op != "=" {
		return
	}
	variable, ok := model.UnwrapParenthesized(target).(*model.Var)
	if !ok {
		return
	}
	if _, ok := model.UnwrapParenthesized(value).(*model.New); !ok {
		return
	}
	if identifiable, ok := result.(interface{ SetID(string) }); ok {
		identifiable.SetID(variable.Name)
	}
}

// propName resolves the property name of an access: the source identifier, or
// the value `$obj->$name` / `$obj->{expr}` supplies.
func (rt *Runtime) propName(tgt *model.PropAccess, scope *Scope) (string, error) {
	if tgt.NameExpr == nil {
		return tgt.Name, nil
	}
	v, err := rt.Eval(tgt.NameExpr, scope)
	if err != nil {
		return "", err
	}
	return phpString(v), nil
}

// assignable reports whether e names storage a write can reach.
func assignable(e model.Expr) bool {
	switch model.UnwrapParenthesized(e).(type) {
	case *model.Var, *model.Index, *model.PropAccess, *model.StaticProp:
		return true
	}
	return false
}

// execUnset removes each target from the scope, array, property bag or static
// bag holding it. Unsetting a name nothing set is not an error, which
// is what lets `unset($map[$key])` run unconditionally.
func (rt *Runtime) execUnset(n *model.Unset, scope *Scope) error {
	for _, target := range n.Targets {
		switch tgt := model.UnwrapParenthesized(target).(type) {
		case *model.Var:
			scope.Unset(tgt.Name)
		case *model.PropAccess:
			base, err := rt.Eval(tgt.Base, scope)
			if err != nil {
				return err
			}
			name, err := rt.propName(tgt, scope)
			if err != nil {
				return err
			}
			if obj, ok := base.(*model.Object); ok {
				obj.DeleteProp(name)
			}
		case *model.StaticProp:
			bag, err := rt.staticStorage(tgt.Class, scope)
			if err != nil {
				return err
			}
			delete(bag, tgt.Name)
		case *model.Index:
			if tgt.Index == nil {
				return fmt.Errorf("unset: [] is not an unset target")
			}
			base, err := rt.Eval(tgt.Base, scope)
			if err != nil {
				return err
			}
			key, err := rt.Eval(tgt.Index, scope)
			if err != nil {
				return err
			}
			if arr, ok := base.(*model.Array); ok {
				arr.Delete(normalizeKey(key))
				continue
			}
			if keyed, ok := base.(model.Keyed); ok {
				keyed.Delete(keyString(key))
				continue
			}
			// A reindexed slice is a new value and it goes back to where the
			// base reads from. A map deleted in place answers nothing.
			if replacement, changed := unsetGoIndex(base, key); changed {
				if err := rt.assignTo(tgt.Base, replacement, scope); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unset: unsupported target %T", target)
		}
	}
	return nil
}

func (rt *Runtime) readLValue(target model.Expr, scope *Scope) (any, error) {
	switch tgt := model.UnwrapParenthesized(target).(type) {
	case *model.Var:
		v, _ := scope.Get(tgt.Name)
		return v, nil
	case *model.PropAccess:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return nil, err
		}
		name, err := rt.propName(tgt, scope)
		if err != nil {
			return nil, err
		}
		obj, ok := base.(*model.Object)
		if !ok {
			return nil, fmt.Errorf("assign: %q is not an object property", name)
		}
		return obj.Props[name], nil
	case *model.StaticProp:
		bag, err := rt.staticStorage(tgt.Class, scope)
		if err != nil {
			return nil, err
		}
		return bag[tgt.Name], nil
	case *model.Index:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return nil, err
		}
		arr, ok := base.(*model.Array)
		if !ok {
			return nil, fmt.Errorf("assign: target is not an array")
		}
		if tgt.Index == nil {
			return nil, fmt.Errorf("assign: [] append target is write-only")
		}
		key, err := rt.Eval(tgt.Index, scope)
		if err != nil {
			return nil, err
		}
		v, _ := arr.Get(normalizeKey(key))
		return v, nil
	default:
		return nil, fmt.Errorf("assign: unsupported target %T", target)
	}
}

// assignTo writes an already-evaluated value into an lvalue (used by list()
// destructuring). Only plain `=` semantics apply here.
func (rt *Runtime) assignTo(target model.Expr, val any, scope *Scope) error {
	switch tgt := model.UnwrapParenthesized(target).(type) {
	case *model.Var:
		rt.setVar(scope, tgt.Name, val)
		return nil
	case *model.PropAccess:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return err
		}
		name, err := rt.propName(tgt, scope)
		if err != nil {
			return err
		}
		return storeProp(base, name, val)
	case *model.StaticProp:
		bag, err := rt.staticStorage(tgt.Class, scope)
		if err != nil {
			return err
		}
		bag[tgt.Name] = val
		return nil
	case *model.Index:
		base, err := rt.container(tgt.Base, scope)
		if err != nil {
			return err
		}
		if tgt.Index == nil {
			arr, ok := base.(*model.Array)
			if !ok {
				return fmt.Errorf("assign: cannot append to %T; a binding whose result is appended to must return *model.Array", base)
			}
			arr.Append(val)
			return nil
		}
		key, err := rt.Eval(tgt.Index, scope)
		if err != nil {
			return err
		}
		return storeIndex(base, key, val)
	default:
		return fmt.Errorf("assign: unsupported list() element %T", target)
	}
}

// applyAssignOp resolves compound-assignment operators against the current
// value. It reports an error for the same inputs the binary operator does and
// `$x <<= -1` fails where `$x = $x << -1` fails, with no silent assignment.
func applyAssignOp(op string, cur, rhs any) (any, error) {
	switch op {
	case "", "=", "[]=":
		return rhs, nil
	case ".=":
		return phpString(cur) + phpString(rhs), nil
	case "+=", "-=", "*=", "/=", "%=", "**=":
		return phpArith(strings.TrimSuffix(op, "="), cur, rhs)
	case "&=", "|=", "^=", "<<=", ">>=":
		return phpBitwise(strings.TrimSuffix(op, "="), cur, rhs)
	default:
		return rhs, nil
	}
}
