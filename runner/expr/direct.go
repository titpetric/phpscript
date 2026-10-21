package expr

import (
	"fmt"
	"strconv"

	"github.com/titpetric/phpscript/model"
)

// This file is the direct compiler's node dispatcher: one case per expression
// shape the runtime evaluates, each compiling to the closure that evaluates
// it. The operator and interpolation sub-compilers it dispatches to live in
// direct_operators.go, and the compiler's bindings and call plumbing in
// compiled.go.

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

// nameOrExpr compiles a runtime name: the constant string when the syntax
// names it, or the expression when a value does (`$obj->$m(...)`,
// `new $class(...)`).
func (dc *directCompiler) nameOrExpr(name string, e model.Expr) (closure, error) {
	if e == nil {
		return constClosure(name), nil
	}
	return dc.compile(e)
}

func (dc *directCompiler) compile(e model.Expr) (closure, error) {
	switch n := e.(type) {
	case *model.Lit:
		// The value goes in as itself, invalid UTF-8 included: with no
		// source text to travel through there is nothing to mis-decode.
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
		// The key and the value closures and no pair closure over them: a pair
		// closure returns model.ArrayItemValue as an any, which boxes a
		// four-word struct on the heap for the line below to unbox again. That
		// is one allocation per entry of every array literal a script writes,
		// and the compiler builds the two slices here once.
		keys := make([]closure, len(n.Items))
		vals := make([]closure, len(n.Items))
		for i, it := range n.Items {
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
		fn := dc.h.Array
		pair := dc.h.Pair
		return func(env *Env) (any, error) {
			items := make([]model.ArrayItemValue, len(vals))
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
				items[i] = pair(key, v)
			}
			return fn(items...), nil
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
		return baseCall("__get", []closure{base, constClosure(n.Name)}), nil

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
		v, ok := model.UnwrapParenthesized(n.Target).(*model.Var)
		if !ok {
			return nil, fmt.Errorf("transpile: assignment expression supports only $var targets, got %T", n.Target)
		}
		// Compound ops in an expression context are rare; support plain `=`.
		if n.Op != "=" && n.Op != "" {
			return nil, fmt.Errorf("transpile: assignment expression op %q unsupported", n.Op)
		}
		val, err := dc.compile(n.Value)
		if err != nil {
			return nil, err
		}
		return baseCall("__set", []closure{constClosure(v.Name), val}), nil

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

// compileCall mirrors Transpiler.emitCall: by-reference output arguments
// that are plain variables become __ref setters, namespaced names and calls
// carrying a global fallback dispatch through __func, and a plain global
// call resolves its name in the base env, installed by Eval.
func (dc *directCompiler) compileCall(n *model.Call) (closure, error) {
	if n.FirstClass {
		// `greet(...)` takes the callable and never calls it, so the name
		// goes in as a value and the fallback with it: resolution is the
		// runtime's, as for a call, and the result is a Closure.
		return baseCall("__callable", []closure{constClosure(n.Name), constClosure(n.Fallback)}), nil
	}
	args := make([]closure, len(n.Args))
	for i, a := range n.Args {
		if model.ByRefArg(n.Name, n.Fallback, i) {
			if v, ok := model.UnwrapParenthesized(a).(*model.Var); ok {
				// The transpiler also registers the variable itself; Eval
				// resolving it keeps the two paths' bindings identical.
				dc.bindVar(v.Name, false)
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
