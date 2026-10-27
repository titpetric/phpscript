package runner

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// This file mutates storage a script names: assignment and unset over every
// lvalue shape, the read an increment makes first, and the compound operators
// both resolve through. The writes that land on a binding's native Go value
// go through assign_go.go.

// execAssign mutates a variable, property or array element. expr-lang cannot do
// this and it lives entirely here.
func (rt *Runtime) execAssign(n *model.Assign, scope *Scope) error {
	rhs, err := rt.Eval(n.Value, scope)
	if err != nil {
		return err
	}
	bindConstructorID(n.Target, n.Value, n.Op, rhs)

	switch tgt := model.UnwrapParenthesized(n.Target).(type) {
	case *model.Var:
		cur, _ := scope.Get(tgt.Name)
		next, err := applyAssignOp(n.Op, cur, rhs)
		if err != nil {
			return err
		}
		rt.setVar(scope, tgt.Name, next)
		return nil

	case *model.ListExpr:
		// list($a, $b) = $arr destructures by position (integer keys). The
		// loop below reads the source through helperIndex and a binding
		// returning a native []string destructures like an *Array.
		if !model.IsCollection(rhs) {
			return fmt.Errorf("assign: list() target requires an array")
		}
		for i, el := range tgt.Elems {
			if el == nil {
				continue
			}
			if err := rt.assignTo(el, helperIndex(rhs, int64(i)), scope); err != nil {
				return err
			}
		}
		return nil

	case *model.PropAccess:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return err
		}
		if obj, ok := base.(*model.Object); ok {
			next, err := applyAssignOp(n.Op, obj.Props[tgt.Name], rhs)
			if err != nil {
				return err
			}
			obj.SetProp(tgt.Name, next)
			return nil
		}
		return assignGoField(base, tgt.Name, func(current any) (any, error) {
			return applyAssignOp(n.Op, current, rhs)
		})

	case *model.StaticProp:
		bag, err := rt.staticStorage(tgt.Class, scope)
		if err != nil {
			return err
		}
		next, err := applyAssignOp(n.Op, bag[tgt.Name], rhs)
		if err != nil {
			return err
		}
		bag[tgt.Name] = next
		return nil

	case *model.Index:
		base, err := rt.container(tgt.Base, scope)
		if err != nil {
			return err
		}
		arr, ok := base.(*model.Array)
		if !ok {
			// Native Go collections are writable where Go itself allows it: a
			// map takes a new or replacement entry, a slice an overwrite of an
			// element it holds. Only `$a[] =` needs an *Array, since a slice
			// cannot grow through the interface holding it.
			if n.Op == "[]=" || tgt.Index == nil {
				return fmt.Errorf("assign: cannot append to %T; a binding whose result is appended to must return *model.Array", base)
			}
			key, err := rt.Eval(tgt.Index, scope)
			if err != nil {
				return err
			}
			return assignGoIndex(base, key, func(current any) (any, error) {
				return applyAssignOp(n.Op, current, rhs)
			})
		}
		if n.Op == "[]=" || tgt.Index == nil {
			arr.Append(rhs)
			return nil
		}
		key, err := rt.Eval(tgt.Index, scope)
		if err != nil {
			return err
		}
		k := normalizeKey(key)
		cur, _ := arr.Get(k)
		next, err := applyAssignOp(n.Op, cur, rhs)
		if err != nil {
			return err
		}
		arr.Set(k, next)
		return nil

	default:
		return fmt.Errorf("assign: unsupported target %T", n.Target)
	}
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

// container evaluates the base of an index assignment, creating the array PHP
// would create when nothing is there yet. `$rows[$first][$second] = $v` against
// an unset $rows makes both levels; container writes the new array back into
// the slot that held null, or the write would land in a value nothing holds.
//
// Only an assignable base is auto-vivified. A function result or a literal has
// nowhere to write the array back to and container returns it as it stands and
// the caller reports the assignment as unsupported.
func (rt *Runtime) container(base model.Expr, scope *Scope) (any, error) {
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
			if obj, ok := base.(*model.Object); ok {
				obj.DeleteProp(tgt.Name)
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
		obj, ok := base.(*model.Object)
		if !ok {
			return nil, fmt.Errorf("assign: %q is not an object property", tgt.Name)
		}
		return obj.Props[tgt.Name], nil
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
		if obj, ok := base.(*model.Object); ok {
			obj.SetProp(tgt.Name, val)
			return nil
		}
		return assignGoField(base, tgt.Name, func(any) (any, error) { return val, nil })
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
		arr, ok := base.(*model.Array)
		if !ok {
			if keyed, ok := base.(model.Keyed); ok && tgt.Index != nil {
				key, err := rt.Eval(tgt.Index, scope)
				if err != nil {
					return err
				}
				keyed.Write(keyString(key), val)
				return nil
			}
			if tgt.Index == nil {
				return fmt.Errorf("assign: cannot append to %T; a binding whose result is appended to must return *model.Array", base)
			}
			key, err := rt.Eval(tgt.Index, scope)
			if err != nil {
				return err
			}
			return assignGoIndex(base, key, func(any) (any, error) { return val, nil })
		}
		if tgt.Index == nil {
			arr.Append(val)
			return nil
		}
		key, err := rt.Eval(tgt.Index, scope)
		if err != nil {
			return err
		}
		arr.Set(normalizeKey(key), val)
		return nil
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
		return phpArith(strings.TrimSuffix(op, "="), cur, rhs), nil
	case "&=", "|=", "^=", "<<=", ">>=":
		return phpBitwise(strings.TrimSuffix(op, "="), cur, rhs)
	default:
		return rhs, nil
	}
}
