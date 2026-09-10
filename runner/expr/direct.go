package expr

import (
	"fmt"
	"strconv"

	"github.com/titpetric/phpscript/model"
)

// BoundVar is one variable the compiled expression reads: the PHP name, the
// bare-name flag Eval's resolution needs, and the slot the value lands in.
type BoundVar struct {
	Name  string
	Const bool
	Slot  int
}

// BoundClosure is one transpiled closure literal: the synthetic identifier,
// the declaration Eval binds a callable for, and its slot.
type BoundClosure struct {
	ID   string
	Slot int
	Decl *model.Closure
}

// Compiled is a model.Expr compiled straight to a closure chain, with the
// bindings Eval fills before running: variables by slot, closure literals by
// slot, and the registered-function names to install into the base env. It
// carries no bytecode; the string round-trip through the transpiler and
// expr-lang's parser never happens for an expression compiled here.
type Compiled struct {
	Program  *Program
	Vars     []BoundVar
	Calls    []string
	Closures []BoundClosure
}

// CompileExpr compiles a model.Expr directly to a closure chain, or reports
// that the tree contains a shape the direct compiler does not cover. The
// decision is all-or-nothing: on false the caller compiles through the
// transpile pipeline, whose closure engine or VM evaluates the expression
// with identical semantics, so falling back loses correctness nothing.
//
// The node coverage mirrors Transpiler.emit case for case; the shapes that
// re-enter the runtime through marked sub-expressions (`++`/`--`, include)
// are the deliberate gaps, since they need the transpiler's expression marks.
func CompileExpr(e model.Expr, h *Helpers) (*Compiled, bool) {
	if h == nil {
		return nil, false
	}
	dc := &directCompiler{h: h, slots: map[string]int{}}
	body, ok := dc.compile(e)
	if !ok {
		return nil, false
	}
	prog := &Program{slots: dc.slots}
	prog.run = wrapRoot(body, h)
	return &Compiled{
		Program:  prog,
		Vars:     dc.vars,
		Calls:    dc.calls,
		Closures: dc.closures,
	}, true
}

type directCompiler struct {
	h        *Helpers
	slots    map[string]int
	vars     []BoundVar
	calls    []string
	callSeen map[string]struct{}
	closures []BoundClosure
}

// bindVar returns the slot for a PHP variable or bare name, registering the
// binding on first sight. The slot key reuses the transpiler's identifier
// spelling (`v_`, `c_`, bare `this`) so one variable referenced twice shares
// one slot.
func (dc *directCompiler) bindVar(name string, isConst bool) int {
	ident := "v_" + name
	if isConst {
		ident = constIdentPrefix + name
	} else if name == "this" {
		ident = "this"
	}
	if slot, ok := dc.slots[ident]; ok {
		return slot
	}
	slot := len(dc.slots)
	dc.slots[ident] = slot
	dc.vars = append(dc.vars, BoundVar{Name: name, Const: isConst, Slot: slot})
	return slot
}

const constIdentPrefix = "c_"

func (dc *directCompiler) addCall(name string) {
	if _, ok := dc.callSeen[name]; ok {
		return
	}
	if dc.callSeen == nil {
		dc.callSeen = map[string]struct{}{}
	}
	dc.callSeen[name] = struct{}{}
	dc.calls = append(dc.calls, name)
}

func slotRead(slot int) closure {
	return func(env *Env) (any, error) {
		return env.Vars[slot], nil
	}
}

// baseCall evaluates args and calls the named base-env entry: a registered
// function installed by Eval, or one of the per-runtime helpers (__call,
// __get, __new, ...). The []any argument slice is their variadic signature.
func baseCall(name string, args []closure) closure {
	return func(env *Env) (any, error) {
		fn, ok := env.Base[name].(func(...any) (any, error))
		if !ok {
			return nil, fmt.Errorf("cannot call %s: not a function", name)
		}
		in := make([]any, len(args))
		for i, c := range args {
			v, err := c(env)
			if err != nil {
				return nil, err
			}
			in[i] = v
		}
		return fn(in...)
	}
}

func (dc *directCompiler) compileArgs(args []model.Expr) ([]closure, bool) {
	out := make([]closure, len(args))
	for i, a := range args {
		c, ok := dc.compile(a)
		if !ok {
			return nil, false
		}
		out[i] = c
	}
	return out, true
}

// nameOrExpr compiles a runtime name: the constant string when the syntax
// names it, or the expression when it is a value (`$obj->$m(...)`,
// `new $class(...)`).
func (dc *directCompiler) nameOrExpr(name string, e model.Expr) (closure, bool) {
	if e == nil {
		return constClosure(name), true
	}
	return dc.compile(e)
}

func (dc *directCompiler) compile(e model.Expr) (closure, bool) {
	switch n := e.(type) {
	case *model.Lit:
		// The value goes in as itself, invalid UTF-8 included: with no
		// source text to travel through there is nothing to mis-decode.
		return constClosure(n.Value), true

	case *model.Var:
		return slotRead(dc.bindVar(n.Name, n.Const)), true

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
		c, ok := dc.compile(n.Cond)
		if !ok {
			return nil, false
		}
		t, ok := dc.compile(n.Then)
		if !ok {
			return nil, false
		}
		f, ok := dc.compile(n.Else)
		if !ok {
			return nil, false
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
		}, true

	case *model.ArrayLit:
		items := make([]closure, len(n.Items))
		for i, it := range n.Items {
			var key closure
			if it.Key != nil {
				k, ok := dc.compile(it.Key)
				if !ok {
					return nil, false
				}
				key = k
			}
			val, ok := dc.compile(it.Val)
			if !ok {
				return nil, false
			}
			items[i] = pairItem(key, val, dc.h.Pair)
		}
		fn := dc.h.Array
		return func(env *Env) (any, error) {
			vals := make([]model.ArrayItemValue, len(items))
			for i, c := range items {
				v, err := c(env)
				if err != nil {
					return nil, err
				}
				vals[i] = v.(model.ArrayItemValue)
			}
			return fn(vals...), nil
		}, true

	case *model.Index:
		base, ok := dc.compile(n.Base)
		if !ok {
			return nil, false
		}
		idx, ok := dc.compile(n.Index)
		if !ok {
			return nil, false
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
		}, true

	case *model.Cast:
		x, ok := dc.compile(n.X)
		if !ok {
			return nil, false
		}
		typ := n.Type
		fn := dc.h.Cast
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return fn(typ, v), nil
		}, true

	case *model.PropAccess:
		base, ok := dc.compile(n.Base)
		if !ok {
			return nil, false
		}
		return baseCall("__get", []closure{base, constClosure(n.Name)}), true

	case *model.Call:
		return dc.compileCall(n)

	case *model.ClassConst:
		return baseCall("__classconst", []closure{constClosure(n.Class), constClosure(n.Name)}), true

	case *model.StaticProp:
		return baseCall("__staticprop", []closure{constClosure(n.Class), constClosure(n.Name)}), true

	case *model.StaticCall:
		method, ok := dc.nameOrExpr(n.Method, n.MethodExpr)
		if !ok {
			return nil, false
		}
		args, ok := dc.compileArgs(n.Args)
		if !ok {
			return nil, false
		}
		return baseCall("__static", append([]closure{constClosure(n.Class), method}, args...)), true

	case *model.MethodCall:
		base, ok := dc.compile(n.Base)
		if !ok {
			return nil, false
		}
		method, ok := dc.nameOrExpr(n.Method, n.MethodExpr)
		if !ok {
			return nil, false
		}
		args, ok := dc.compileArgs(n.Args)
		if !ok {
			return nil, false
		}
		return baseCall("__call", append([]closure{base, method}, args...)), true

	case *model.New:
		class, ok := dc.nameOrExpr(n.Class, n.ClassExpr)
		if !ok {
			return nil, false
		}
		args, ok := dc.compileArgs(n.Args)
		if !ok {
			return nil, false
		}
		return baseCall("__new", append([]closure{class}, args...)), true

	case *model.Invoke:
		callee, ok := dc.compile(n.Callee)
		if !ok {
			return nil, false
		}
		args, ok := dc.compileArgs(n.Args)
		if !ok {
			return nil, false
		}
		return baseCall("__invoke", append([]closure{callee}, args...)), true

	case *model.AssignExpr:
		v, ok := model.UnwrapParenthesized(n.Target).(*model.Var)
		if !ok || (n.Op != "=" && n.Op != "") {
			// The transpiler rejects these too; the fallback reports the
			// same error.
			return nil, false
		}
		val, ok := dc.compile(n.Value)
		if !ok {
			return nil, false
		}
		return baseCall("__set", []closure{constClosure(v.Name), val}), true

	case *model.Closure:
		id := "__cl" + strconv.Itoa(len(dc.closures))
		slot := len(dc.slots)
		dc.slots[id] = slot
		dc.closures = append(dc.closures, BoundClosure{ID: id, Slot: slot, Decl: n})
		return slotRead(slot), true
	}
	return nil, false
}

// pairItem builds one __pair value; a nil key is a list-style append.
func pairItem(key, val closure, pair func(key, val any) model.ArrayItemValue) closure {
	return func(env *Env) (any, error) {
		var k any
		if key != nil {
			var err error
			if k, err = key(env); err != nil {
				return nil, err
			}
		}
		v, err := val(env)
		if err != nil {
			return nil, err
		}
		return pair(k, v), nil
	}
}

// compileInterp folds an interpolated string over Concat, matching
// Transpiler.emitInterp: a lone literal part is itself, a lone expression
// part concatenates with "" so the result is a string either way.
func (dc *directCompiler) compileInterp(n *model.Interp) (closure, bool) {
	if len(n.Parts) == 0 {
		return constClosure(""), true
	}
	out, ok := dc.compile(n.Parts[0])
	if !ok {
		return nil, false
	}
	fn := dc.h.Concat
	if len(n.Parts) == 1 {
		if _, isLit := n.Parts[0].(*model.Lit); isLit {
			return out, true
		}
		part := out
		return func(env *Env) (any, error) {
			v, err := part(env)
			if err != nil {
				return nil, err
			}
			return fn("", v), nil
		}, true
	}
	for _, p := range n.Parts[1:] {
		next, ok := dc.compile(p)
		if !ok {
			return nil, false
		}
		left, right := out, next
		out = func(env *Env) (any, error) {
			a, err := left(env)
			if err != nil {
				return nil, err
			}
			b, err := right(env)
			if err != nil {
				return nil, err
			}
			return fn(a, b), nil
		}
	}
	return out, true
}

func (dc *directCompiler) compileUnary(n *model.Unary) (closure, bool) {
	// `++`/`--` re-enter the runtime through a transpiler mark; anything
	// else the transpiler passes to expr-lang as a bare operator falls back
	// with it.
	switch n.Op {
	case "!", "~", "-":
	default:
		return nil, false
	}
	x, ok := dc.compile(n.X)
	if !ok {
		return nil, false
	}
	switch n.Op {
	case "!":
		truthy := dc.h.Truthy
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return !truthy(v), nil
		}, true
	case "~":
		fn := dc.h.BitNot
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return fn(v), nil
		}, true
	default:
		fn := dc.h.Negate
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return fn(v), nil
		}, true
	}
}

func (dc *directCompiler) compileBinary(n *model.Binary) (closure, bool) {
	// `instanceof` with a bare name on the right is the class, not a
	// constant to resolve.
	if n.Op == "instanceof" {
		if v, ok := model.UnwrapParenthesized(n.Right).(*model.Var); ok && v.Const {
			l, ok := dc.compile(n.Left)
			if !ok {
				return nil, false
			}
			fn := dc.h.InstanceOf
			name := v.Name
			return func(env *Env) (any, error) {
				a, err := l(env)
				if err != nil {
					return nil, err
				}
				return fn(a, name), nil
			}, true
		}
	}

	l, ok := dc.compile(n.Left)
	if !ok {
		return nil, false
	}
	r, ok := dc.compile(n.Right)
	if !ok {
		return nil, false
	}

	switch n.Op {
	case "&&", "||":
		truthy := dc.h.Truthy
		and := n.Op == "&&"
		return func(env *Env) (any, error) {
			a, err := l(env)
			if err != nil {
				return nil, err
			}
			if truthy(a) != and {
				return !and, nil
			}
			b, err := r(env)
			if err != nil {
				return nil, err
			}
			return truthy(b), nil
		}, true
	case ".":
		fn := dc.h.Concat
		return binary2(l, r, func(a, b any) (any, error) { return fn(a, b), nil }), true
	case "instanceof":
		fn := dc.h.InstanceOf
		return binary2(l, r, func(a, b any) (any, error) { return fn(a, b), nil }), true
	case "+", "-", "*", "/", "%", "**":
		fn := dc.h.Arith
		op := n.Op
		return binary2(l, r, func(a, b any) (any, error) { return fn(op, a, b), nil }), true
	case "&", "|", "^", "<<", ">>":
		fn := dc.h.Bitwise
		op := n.Op
		return binary2(l, r, func(a, b any) (any, error) { return fn(op, a, b) }), true
	case "==", "!=", "===", "!==", "<", "<=", ">", ">=":
		fn := dc.h.Compare
		op := n.Op
		return binary2(l, r, func(a, b any) (any, error) { return fn(op, a, b), nil }), true
	}
	return nil, false
}

func binary2(l, r closure, apply func(a, b any) (any, error)) closure {
	return func(env *Env) (any, error) {
		a, err := l(env)
		if err != nil {
			return nil, err
		}
		b, err := r(env)
		if err != nil {
			return nil, err
		}
		return apply(a, b)
	}
}

// compileCall mirrors Transpiler.emitCall: by-reference output arguments
// that are plain variables become __ref setters, namespaced names and calls
// carrying a global fallback dispatch through __func, and a plain global
// call resolves its name in the base env, installed by Eval.
func (dc *directCompiler) compileCall(n *model.Call) (closure, bool) {
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
		c, ok := dc.compile(a)
		if !ok {
			return nil, false
		}
		args[i] = c
	}
	if n.Fallback != "" || containsBackslash(n.Name) {
		return baseCall("__func", append([]closure{constClosure(n.Name), constClosure(n.Fallback)}, args...)), true
	}
	dc.addCall(n.Name)
	return baseCall(n.Name, args), true
}

func containsBackslash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			return true
		}
	}
	return false
}
