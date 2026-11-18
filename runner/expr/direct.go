package expr

import (
	"fmt"
	"strconv"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// This file is the direct compiler's node dispatcher: one case per expression
// shape the runtime evaluates, each compiling to a closure that evaluates the
// shape. The operator and interpolation sub-compilers it dispatches to are in
// direct_operators.go, and the compiler's bindings and call plumbing in
// compiled.go.

// compileLitEntries compiles a literal's entries to one closure returning the evaluated pairs.
func (dc *directCompiler) compileLitEntries(items []model.ArrayItem) (func(*Env) ([]model.ArrayItemValue, error), error) {
	keys := make([]closure, len(items))
	vals := make([]closure, len(items))
	for i, it := range items {
		if it.Key != nil {
			key, err := dc.compile(it.Key)
			if err != nil {
				return nil, err
			}
			keys[i] = key
		}
		val, err := dc.compile(it.Val)
		if err != nil {
			return nil, err
		}
		vals[i] = val
	}
	pair := dc.h.Pair
	return func(env *Env) ([]model.ArrayItemValue, error) {
		out := make([]model.ArrayItemValue, len(vals))
		for i, val := range vals {
			// The key ahead of the value, entry by entry, which is the
			// order php evaluates a literal in. A nil key is a
			// list-style append.
			var key any
			if keys[i] != nil {
				k, err := keys[i](env)
				if err != nil {
					return nil, err
				}
				key = k
			}
			v, err := val(env)
			if err != nil {
				return nil, err
			}
			out[i] = pair(key, v)
		}
		return out, nil
	}, nil
}

func (dc *directCompiler) compileArgs(args []model.Expr) ([]closure, error) {
	out := make([]closure, len(args))
	for i, a := range args {
		c, err := dc.compile(a)
		if err != nil {
			return nil, err
		}
		out[i] = c
	}
	return out, nil
}

// nameOrExpr compiles the constant name, or the expression when a value names it.
func (dc *directCompiler) nameOrExpr(name string, e model.Expr) (closure, error) {
	if e == nil {
		return constClosure(name), nil
	}
	return dc.compile(e)
}

func (dc *directCompiler) compile(e model.Expr) (closure, error) {
	switch n := e.(type) {
	case *model.Lit:
		// The value goes in unchanged, invalid UTF-8 included. No source text
		// travels with the value. No decoding can corrupt it.
		return constClosure(n.Value), nil

	case *model.Var:
		if dc.liveVars {
			return baseCall("__var", []closure{constClosure(n.Name), constClosure(n.Const)}), nil
		}
		return slotRead(dc.bindVar(n.Name, n.Const)), nil

	case *model.Parenthesized:
		return dc.compile(n.X)

	case *model.Ref:
		// `&$var` binds by value, as in the transpiler.
		return dc.compile(n.X)

	case *model.Interp:
		return dc.compileInterp(n)

	case *model.Unary:
		return dc.compileUnary(n)

	case *model.Binary:
		return dc.compileBinary(n)

	case *model.Coalesce:
		l, err := dc.compile(n.Left)
		if err != nil {
			return nil, err
		}
		r, err := dc.compile(n.Right)
		if err != nil {
			return nil, err
		}
		return func(env *Env) (any, error) {
			v, err := l(env)
			if err != nil || v != nil {
				return v, err
			}
			return r(env)
		}, nil

	case *model.Ternary:
		c, err := dc.compile(n.Cond)
		if err != nil {
			return nil, err
		}
		t, err := dc.compile(n.Then)
		if err != nil {
			return nil, err
		}
		f, err := dc.compile(n.Else)
		if err != nil {
			return nil, err
		}
		truthy := dc.h.Truthy
		return func(env *Env) (any, error) {
			v, err := c(env)
			if err != nil {
				return nil, err
			}
			if truthy(v) {
				return t(env)
			}
			return f(env)
		}, nil

	case *model.ArrayLit:
		entries, err := dc.compileLitEntries(n.Items)
		if err != nil {
			return nil, err
		}
		fn := dc.h.Array
		return func(env *Env) (any, error) {
			items, err := entries(env)
			if err != nil {
				return nil, err
			}
			return fn(items...), nil
		}, nil

	case *model.GoLit:
		entries, err := dc.compileLitEntries(n.Items)
		if err != nil {
			return nil, err
		}
		fn := dc.h.GoLit
		mapLit, keyType, valType := n.Map, n.KeyType, n.ValType
		return func(env *Env) (any, error) {
			items, err := entries(env)
			if err != nil {
				return nil, err
			}
			return fn(mapLit, keyType, valType, items...)
		}, nil

	case *model.Index:
		base, err := dc.compile(n.Base)
		if err != nil {
			return nil, err
		}
		idx, err := dc.compile(n.Index)
		if err != nil {
			return nil, err
		}
		fn := dc.h.Index
		return func(env *Env) (any, error) {
			b, err := base(env)
			if err != nil {
				return nil, err
			}
			i, err := idx(env)
			if err != nil {
				return nil, err
			}
			return fn(b, i), nil
		}, nil

	case *model.Cast:
		x, err := dc.compile(n.X)
		if err != nil {
			return nil, err
		}
		typ := n.Type
		fn := dc.h.Cast
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return fn(typ, v), nil
		}, nil

	case *model.PropAccess:
		base, err := dc.compile(n.Base)
		if err != nil {
			return nil, err
		}
		name, err := dc.nameOrExpr(n.Name, n.NameExpr)
		if err != nil {
			return nil, err
		}
		return baseCall("__get", []closure{base, name}), nil

	case *model.Call:
		return dc.compileCall(n)

	case *model.ClassConst:
		return baseCall("__classconst", []closure{constClosure(n.Class), constClosure(n.Name)}), nil

	case *model.StaticProp:
		return baseCall("__staticprop", []closure{constClosure(n.Class), constClosure(n.Name)}), nil

	case *model.StaticCall:
		method, err := dc.nameOrExpr(n.Method, n.MethodExpr)
		if err != nil {
			return nil, err
		}
		if n.FirstClass {
			return baseCall("__callablemember", []closure{constClosure(n.Class), method}), nil
		}
		args, err := dc.compileArgs(n.Args)
		if err != nil {
			return nil, err
		}
		return baseCall("__static", append([]closure{constClosure(n.Class), method}, args...)), nil

	case *model.MethodCall:
		base, err := dc.compile(n.Base)
		if err != nil {
			return nil, err
		}
		method, err := dc.nameOrExpr(n.Method, n.MethodExpr)
		if err != nil {
			return nil, err
		}
		if n.FirstClass {
			return baseCall("__callablemember", []closure{base, method}), nil
		}
		args, err := dc.compileArgs(n.Args)
		if err != nil {
			return nil, err
		}
		return baseCall("__call", append([]closure{base, method}, args...)), nil

	case *model.New:
		class, err := dc.nameOrExpr(n.Class, n.ClassExpr)
		if err != nil {
			return nil, err
		}
		args, err := dc.compileArgs(n.Args)
		if err != nil {
			return nil, err
		}
		return baseCall("__new", append([]closure{class}, args...)), nil

	case *model.Invoke:
		callee, err := dc.compile(n.Callee)
		if err != nil {
			return nil, err
		}
		if n.FirstClass {
			return baseCall("__callable", []closure{callee, constClosure("")}), nil
		}
		args, err := dc.compileArgs(n.Args)
		if err != nil {
			return nil, err
		}
		return baseCall("__invoke", append([]closure{callee}, args...)), nil

	case *model.AssignExpr:
		v, isVar := modelutil.UnwrapParenthesized(n.Target).(*model.Var)
		if isVar && (n.Op == "=" || n.Op == "") {
			val, err := dc.compile(n.Value)
			if err != nil {
				return nil, err
			}
			return baseCall("__set", []closure{constClosure(v.Name), val}), nil
		}
		// An element, a property, a compound operator: each writes storage a
		// closure environment cannot name. The runtime stores through the node
		// and answers the stored value. See runner.assignValue.
		return dc.markCall(n), nil

	case *model.AddrOf, *model.Deref:
		// A pointer reads and writes storage a closure environment cannot
		// name: the runtime evaluates the node with the live scope.
		return dc.markCall(n), nil

	case *model.Include:
		return dc.markCall(n), nil

	case *model.Closure:
		id := "__cl" + strconv.Itoa(len(dc.closures))
		slot := len(dc.slots)
		dc.slots[id] = slot
		dc.closures = append(dc.closures, BoundClosure{ID: id, Slot: slot, Decl: n})
		return slotRead(slot), nil
	}
	return nil, fmt.Errorf("transpile: unsupported expression %T", e)
}

// compileCall mirrors emitCall: __ref output args, __func dispatch and global fallback.
func (dc *directCompiler) compileCall(n *model.Call) (closure, error) {
	if n.FirstClass {
		// `greet(...)` takes the callable without calling it. Resolution is
		// the runtime's, as for a call. The name and the fallback go in as
		// values. The result is a Closure.
		return baseCall("__callable", []closure{constClosure(n.Name), constClosure(n.Fallback)}), nil
	}
	args := make([]closure, len(n.Args))
	for i, a := range n.Args {
		if modelutil.ByRefArg(n.Name, n.Fallback, i) {
			if v, ok := modelutil.UnwrapParenthesized(a).(*model.Var); ok {
				// The transpiler also registers the variable itself; Eval
				// resolving it keeps the two paths' bindings identical.
				dc.bindVar(v.Name, false)
				dc.outParams = true
				args[i] = baseCall("__ref", []closure{constClosure(v.Name)})
				continue
			}
		}
		c, err := dc.compile(a)
		if err != nil {
			return nil, err
		}
		args[i] = c
	}
	if n.Fallback != "" || containsBackslash(n.Name) {
		return baseCall("__func", append([]closure{constClosure(n.Name), constClosure(n.Fallback)}, args...)), nil
	}
	dc.addCall(n.Name)
	return baseCall(n.Name, args), nil
}

func containsBackslash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			return true
		}
	}
	return false
}
