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

// BoundClosure is one compiled closure literal: the synthetic identifier,
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
	// Exprs holds the marked sub-expressions (`++`/`--`, include) the
	// compiled closures evaluate through the __eval base helper, keyed by
	// the mark id, the same contract the transpiler's marks use.
	Exprs map[string]model.Expr
}

// CompileExpr compiles a model.Expr directly to a closure chain. It covers
// the whole expression vocabulary the runtime evaluates; an expression shape
// outside it is an error, spelled the way the transpiler spelled it when it
// was the compiler.
func CompileExpr(e model.Expr, h *Helpers) (*Compiled, error) {
	dc := &directCompiler{h: h, slots: map[string]int{}}
	body, err := dc.compile(e)
	if err != nil {
		return nil, err
	}
	if len(dc.exprs) > 0 && len(dc.vars) > 0 {
		// A marked sub-expression writes the scope mid-evaluation, and a
		// slot snapshot taken before the run would miss it: $w++ . $w must
		// read the incremented $w. Recompile with variables read live
		// through the __var helper.
		dc = &directCompiler{h: h, liveVars: true, slots: map[string]int{}}
		if body, err = dc.compile(e); err != nil {
			return nil, err
		}
	}
	prog := &Program{slots: dc.slots}
	prog.run = wrapRoot(body, h)
	return &Compiled{
		Program:  prog,
		Vars:     dc.vars,
		Calls:    dc.calls,
		Closures: dc.closures,
		Exprs:    dc.exprs,
	}, nil
}

type directCompiler struct {
	h        *Helpers
	liveVars bool
	slots    map[string]int
	vars     []BoundVar
	calls    []string
	callSeen map[string]struct{}
	closures []BoundClosure
	exprs    map[string]model.Expr
}

// markCall registers e as a marked sub-expression and compiles to an __eval
// call on its id: the runtime evaluates the node itself, with live scope
// reads and lvalue writes the closure environment cannot express.
func (dc *directCompiler) markCall(e model.Expr) closure {
	if dc.exprs == nil {
		dc.exprs = map[string]model.Expr{}
	}
	id := "__m" + strconv.Itoa(len(dc.exprs))
	dc.exprs[id] = e
	return baseCall("__eval", []closure{constClosure(id)})
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
// names it, or the expression when it is a value (`$obj->$m(...)`,
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
		items := make([]closure, len(n.Items))
		for i, it := range n.Items {
			var key closure
			if it.Key != nil {
				k, err := dc.compile(it.Key)
				if err != nil {
					return nil, err
				}
				key = k
			}
			val, err := dc.compile(it.Val)
			if err != nil {
				return nil, err
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
func (dc *directCompiler) compileInterp(n *model.Interp) (closure, error) {
	if len(n.Parts) == 0 {
		return constClosure(""), nil
	}
	out, err := dc.compile(n.Parts[0])
	if err != nil {
		return nil, err
	}
	fn := dc.h.Concat
	if len(n.Parts) == 1 {
		if _, isLit := n.Parts[0].(*model.Lit); isLit {
			return out, nil
		}
		part := out
		return func(env *Env) (any, error) {
			v, err := part(env)
			if err != nil {
				return nil, err
			}
			return fn("", v), nil
		}, nil
	}
	for _, p := range n.Parts[1:] {
		next, err := dc.compile(p)
		if err != nil {
			return nil, err
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
	return out, nil
}

func (dc *directCompiler) compileUnary(n *model.Unary) (closure, error) {
	switch n.Op {
	case "!", "~", "-", "+":
	case "++", "--":
		// The value must be read from the live scope at evaluation time,
		// not from the slot snapshot, and the target is a general lvalue:
		// the mark re-enters the runtime, exactly as the transpiled form
		// does through __eval.
		return dc.markCall(n), nil
	default:
		return nil, fmt.Errorf("transpile: unsupported operator %q", n.Op)
	}
	x, err := dc.compile(n.X)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case "+":
		// PHP's unary plus is the numeric cast 0 + $x.
		fn := dc.h.Arith
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return fn("+", int64(0), v), nil
		}, nil
	case "!":
		truthy := dc.h.Truthy
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return !truthy(v), nil
		}, nil
	case "~":
		fn := dc.h.BitNot
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return fn(v), nil
		}, nil
	default:
		fn := dc.h.Negate
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return fn(v), nil
		}, nil
	}
}

func (dc *directCompiler) compileBinary(n *model.Binary) (closure, error) {
	// `instanceof` with a bare name on the right is the class, not a
	// constant to resolve.
	if n.Op == "instanceof" {
		if v, ok := model.UnwrapParenthesized(n.Right).(*model.Var); ok && v.Const {
			l, err := dc.compile(n.Left)
			if err != nil {
				return nil, err
			}
			fn := dc.h.InstanceOf
			name := v.Name
			return func(env *Env) (any, error) {
				a, err := l(env)
				if err != nil {
					return nil, err
				}
				return fn(a, name), nil
			}, nil
		}
	}

	l, err := dc.compile(n.Left)
	if err != nil {
		return nil, err
	}
	r, err := dc.compile(n.Right)
	if err != nil {
		return nil, err
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
		}, nil
	case ".":
		fn := dc.h.Concat
		return binary2(l, r, func(a, b any) (any, error) { return fn(a, b), nil }), nil
	case "instanceof":
		fn := dc.h.InstanceOf
		return binary2(l, r, func(a, b any) (any, error) { return fn(a, b), nil }), nil
	case "+", "-", "*", "/", "%", "**":
		fn := dc.h.Arith
		op := n.Op
		return binary2(l, r, func(a, b any) (any, error) { return fn(op, a, b), nil }), nil
	case "&", "|", "^", "<<", ">>":
		fn := dc.h.Bitwise
		op := n.Op
		return binary2(l, r, func(a, b any) (any, error) { return fn(op, a, b) }), nil
	case "==", "!=", "===", "!==", "<", "<=", ">", ">=":
		fn := dc.h.Compare
		op := n.Op
		return binary2(l, r, func(a, b any) (any, error) { return fn(op, a, b), nil }), nil
	}
	return nil, fmt.Errorf("transpile: unsupported operator %q", n.Op)
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
func (dc *directCompiler) compileCall(n *model.Call) (closure, error) {
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
