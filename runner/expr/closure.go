package expr

import (
	"fmt"

	"github.com/expr-lang/expr/ast"

	"github.com/titpetric/phpscript/model"
)

// Env is the closure engine's evaluation environment. Per-expression values
// (PHP variables, bare-name constants, transpiled closures) live in Vars,
// indexed by the slot the compiler assigned to their identifier; everything
// persistent (the PHP-semantic helpers, installed functions) stays in Base,
// which is the same map the VM path layers over. Slots let Eval skip the
// per-evaluation map writes and deletes the VM path pays.
type Env struct {
	Vars []any
	Base map[string]any
}

// closure evaluates one node against the runner's evaluation environment.
type closure = func(env *Env) (any, error)

// Helpers carries the typed implementations of the PHP-semantic helper
// functions the transpiler emits. The closure engine calls them directly,
// without the []any argument slice, the adapt() indirection or the per-call
// panic guard the VM's env dispatch pays. Only the pure helpers belong here;
// anything that re-enters the interpreter (__call, __get, __new, registered
// functions) stays an env lookup, because those are per-runtime closures.
type Helpers struct {
	Truthy     func(v any) bool
	Concat     func(a, b any) string
	Pair       func(key, val any) model.ArrayItemValue
	Array      func(items ...model.ArrayItemValue) *model.Array
	Index      func(base, idx any) any
	Cast       func(typ string, v any) any
	Arith      func(op string, a, b any) any
	Compare    func(op string, a, b any) bool
	Bitwise    func(op string, a, b any) (any, error)
	BitNot     func(v any) any
	InstanceOf func(value, class any) bool
	Negate     func(v any) any

	// PanicError converts a recovered panic into the error the runner's own
	// call boundary would have produced, so a host panic stays catchable as
	// the same PHP exception on both engines.
	PanicError func(recovered any) error

	// Slotted reports whether an identifier is a per-evaluation value the
	// runner would have layered into the env map: a PHP variable, a bare
	// name, a transpiled closure. Those compile to slot reads; everything
	// else (function names) stays a Base lookup.
	Slotted func(name string) bool
}

// closureCompiler carries the compile state: the helper table and the slot
// index it assigns to each per-evaluation identifier as it meets one.
type closureCompiler struct {
	h     *Helpers
	slots map[string]int
}

func (cc *closureCompiler) slot(name string) int {
	if i, ok := cc.slots[name]; ok {
		return i
	}
	i := len(cc.slots)
	cc.slots[name] = i
	return i
}

// compileClosure builds the closure chain for a checked, optimized tree and
// the slot table its identifiers resolve through, or reports that the tree
// contains a shape the engine does not recognise. The decision is
// all-or-nothing per expression: a nil return means the whole program runs
// on the VM, never a mix.
func compileClosure(node ast.Node, h *Helpers) (closure, map[string]int) {
	cc := &closureCompiler{h: h, slots: map[string]int{}}
	body, ok := cc.compileNode(node)
	if !ok {
		return nil, nil
	}
	run := func(env *Env) (out any, err error) {
		// One guard per evaluation instead of the VM path's one per helper
		// call. Both engines surface a host panic as an error; PanicError
		// keeps the error type identical.
		defer func() {
			if recovered := recover(); recovered != nil {
				out = nil
				if h.PanicError != nil {
					err = h.PanicError(recovered)
				} else {
					err = fmt.Errorf("panic: %v", recovered)
				}
			}
		}()
		return body(env)
	}
	return run, cc.slots
}

// constClosure returns v itself: literals are boxed once at compile time.
func constClosure(v any) closure {
	return func(*Env) (any, error) { return v, nil }
}

// boolOperand evaluates c and asserts the result is a bool, which is what the
// VM's jump and not opcodes do. The transpiler wraps every logical operand in
// __bool, so the assertion only fails where the VM would have failed too.
func boolOperand(c closure, env *Env) (bool, error) {
	v, err := c(env)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("invalid operation: expected bool, got %T", v)
	}
	return b, nil
}

func (cc *closureCompiler) compileNode(node ast.Node) (closure, bool) {
	switch n := node.(type) {
	case *ast.NilNode:
		return constClosure(nil), true
	case *ast.IntegerNode:
		return constClosure(n.Value), true
	case *ast.FloatNode:
		return constClosure(n.Value), true
	case *ast.BoolNode:
		return constClosure(n.Value), true
	case *ast.StringNode:
		return constClosure(n.Value), true
	case *ast.ConstantNode:
		return constClosure(n.Value), true

	case *ast.IdentifierNode:
		name := n.Value
		if cc.h.Slotted != nil && cc.h.Slotted(name) {
			slot := cc.slot(name)
			return func(env *Env) (any, error) {
				return env.Vars[slot], nil
			}, true
		}
		return func(env *Env) (any, error) {
			return env.Base[name], nil
		}, true

	case *ast.UnaryNode:
		if n.Operator != "!" && n.Operator != "not" {
			return nil, false
		}
		x, ok := cc.compileNode(n.Node)
		if !ok {
			return nil, false
		}
		return func(env *Env) (any, error) {
			b, err := boolOperand(x, env)
			if err != nil {
				return nil, err
			}
			return !b, nil
		}, true

	case *ast.BinaryNode:
		l, ok := cc.compileNode(n.Left)
		if !ok {
			return nil, false
		}
		r, ok := cc.compileNode(n.Right)
		if !ok {
			return nil, false
		}
		switch n.Operator {
		case "&&", "and":
			return func(env *Env) (any, error) {
				lb, err := boolOperand(l, env)
				if err != nil {
					return nil, err
				}
				if !lb {
					return false, nil
				}
				rb, err := boolOperand(r, env)
				if err != nil {
					return nil, err
				}
				return rb, nil
			}, true
		case "||", "or":
			return func(env *Env) (any, error) {
				lb, err := boolOperand(l, env)
				if err != nil {
					return nil, err
				}
				if lb {
					return true, nil
				}
				rb, err := boolOperand(r, env)
				if err != nil {
					return nil, err
				}
				return rb, nil
			}, true
		}
		return nil, false

	case *ast.ConditionalNode:
		c, ok := cc.compileNode(n.Cond)
		if !ok {
			return nil, false
		}
		t, ok := cc.compileNode(n.Exp1)
		if !ok {
			return nil, false
		}
		f, ok := cc.compileNode(n.Exp2)
		if !ok {
			return nil, false
		}
		return func(env *Env) (any, error) {
			b, err := boolOperand(c, env)
			if err != nil {
				return nil, err
			}
			if b {
				return t(env)
			}
			return f(env)
		}, true

	case *ast.CallNode:
		return cc.compileCall(n)
	}
	return nil, false
}

// opArg reads the constant operator string the transpiler bakes into the
// first argument of __arith, __cmp, __bit and __cast calls.
func opArg(n ast.Node) (string, bool) {
	s, ok := n.(*ast.StringNode)
	if !ok {
		return "", false
	}
	return s.Value, true
}

func (cc *closureCompiler) compileCall(n *ast.CallNode) (closure, bool) {
	callee, ok := n.Callee.(*ast.IdentifierNode)
	if !ok {
		return nil, false
	}
	name := callee.Value

	// The typed helpers, dispatched at compile time. Arity is fixed by the
	// transpiler; a call that does not match falls back to the VM rather
	// than guessing.
	switch name {
	case "__bool":
		if len(n.Arguments) == 1 {
			x, ok := cc.compileNode(n.Arguments[0])
			if !ok {
				return nil, false
			}
			fn := cc.h.Truthy
			return func(env *Env) (any, error) {
				v, err := x(env)
				if err != nil {
					return nil, err
				}
				return fn(v), nil
			}, true
		}
		return nil, false

	case "__arith", "__cmp", "__bit":
		if len(n.Arguments) != 3 {
			return nil, false
		}
		op, ok := opArg(n.Arguments[0])
		if !ok {
			return nil, false
		}
		l, ok := cc.compileNode(n.Arguments[1])
		if !ok {
			return nil, false
		}
		r, ok := cc.compileNode(n.Arguments[2])
		if !ok {
			return nil, false
		}
		switch name {
		case "__arith":
			fn := cc.h.Arith
			return func(env *Env) (any, error) {
				a, err := l(env)
				if err != nil {
					return nil, err
				}
				b, err := r(env)
				if err != nil {
					return nil, err
				}
				return fn(op, a, b), nil
			}, true
		case "__cmp":
			fn := cc.h.Compare
			return func(env *Env) (any, error) {
				a, err := l(env)
				if err != nil {
					return nil, err
				}
				b, err := r(env)
				if err != nil {
					return nil, err
				}
				return fn(op, a, b), nil
			}, true
		default:
			fn := cc.h.Bitwise
			return func(env *Env) (any, error) {
				a, err := l(env)
				if err != nil {
					return nil, err
				}
				b, err := r(env)
				if err != nil {
					return nil, err
				}
				return fn(op, a, b)
			}, true
		}

	case "__concat", "__index", "__instanceof", "__pair":
		if len(n.Arguments) != 2 {
			return nil, false
		}
		l, ok := cc.compileNode(n.Arguments[0])
		if !ok {
			return nil, false
		}
		r, ok := cc.compileNode(n.Arguments[1])
		if !ok {
			return nil, false
		}
		switch name {
		case "__concat":
			fn := cc.h.Concat
			return func(env *Env) (any, error) {
				a, err := l(env)
				if err != nil {
					return nil, err
				}
				b, err := r(env)
				if err != nil {
					return nil, err
				}
				return fn(a, b), nil
			}, true
		case "__index":
			fn := cc.h.Index
			return func(env *Env) (any, error) {
				a, err := l(env)
				if err != nil {
					return nil, err
				}
				b, err := r(env)
				if err != nil {
					return nil, err
				}
				return fn(a, b), nil
			}, true
		case "__instanceof":
			fn := cc.h.InstanceOf
			return func(env *Env) (any, error) {
				a, err := l(env)
				if err != nil {
					return nil, err
				}
				b, err := r(env)
				if err != nil {
					return nil, err
				}
				return fn(a, b), nil
			}, true
		default:
			fn := cc.h.Pair
			return func(env *Env) (any, error) {
				a, err := l(env)
				if err != nil {
					return nil, err
				}
				b, err := r(env)
				if err != nil {
					return nil, err
				}
				return fn(a, b), nil
			}, true
		}

	case "__neg", "__bitnot":
		if len(n.Arguments) != 1 {
			return nil, false
		}
		x, ok := cc.compileNode(n.Arguments[0])
		if !ok {
			return nil, false
		}
		fn := cc.h.Negate
		if name == "__bitnot" {
			fn = cc.h.BitNot
		}
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return fn(v), nil
		}, true

	case "__cast":
		if len(n.Arguments) != 2 {
			return nil, false
		}
		typ, ok := opArg(n.Arguments[0])
		if !ok {
			return nil, false
		}
		x, ok := cc.compileNode(n.Arguments[1])
		if !ok {
			return nil, false
		}
		fn := cc.h.Cast
		return func(env *Env) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return fn(typ, v), nil
		}, true

	case "__array":
		items := make([]closure, len(n.Arguments))
		for i, a := range n.Arguments {
			c, ok := cc.compileNode(a)
			if !ok {
				return nil, false
			}
			items[i] = c
		}
		fn := cc.h.Array
		return func(env *Env) (any, error) {
			vals := make([]model.ArrayItemValue, len(items))
			for i, c := range items {
				v, err := c(env)
				if err != nil {
					return nil, err
				}
				item, ok := v.(model.ArrayItemValue)
				if !ok {
					return nil, fmt.Errorf("__array: expected array item, got %T", v)
				}
				vals[i] = item
			}
			return fn(vals...), nil
		}, true
	}

	// Everything else (registered functions, the interpreter-re-entering
	// helpers, transpiled closures) is a per-runtime value in the eval env,
	// installed by Eval for exactly the names the expression calls. The
	// argument slice is inherent to their variadic signature; what the
	// closure path drops is the VM's own dispatch around it.
	args := make([]closure, len(n.Arguments))
	for i, a := range n.Arguments {
		c, ok := cc.compileNode(a)
		if !ok {
			return nil, false
		}
		args[i] = c
	}
	slotted := cc.h.Slotted != nil && cc.h.Slotted(name)
	slot := -1
	if slotted {
		slot = cc.slot(name)
	}
	return func(env *Env) (any, error) {
		var callee any
		if slotted {
			callee = env.Vars[slot]
		} else {
			callee = env.Base[name]
		}
		fn, ok := callee.(func(...any) (any, error))
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
	}, true
}
