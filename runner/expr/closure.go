package expr

import (
	"fmt"

	"github.com/expr-lang/expr/ast"

	"github.com/titpetric/phpscript/model"
)

// closure evaluates one node against the runner's evaluation environment.
type closure = func(env map[string]any) (any, error)

// Helpers carries the typed implementations of the PHP-semantic helper
// functions the transpiler emits. The closure engine calls them directly,
// without the []any argument slice, the adapt() indirection or the per-call
// panic guard the VM's env dispatch pays; that is the whole point of the
// engine. Only the pure helpers belong here — anything that re-enters the
// interpreter (__call, __get, __new, registered functions) stays an env
// lookup, because those are per-runtime closures.
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
}

// compileClosure builds the closure chain for a checked, optimized tree, or
// reports that the tree contains a shape the engine does not recognise. The
// decision is all-or-nothing per expression: a nil return means the whole
// program runs on the VM, never a mix.
func compileClosure(node ast.Node, h *Helpers) closure {
	body, ok := compileNode(node, h)
	if !ok {
		return nil
	}
	return func(env map[string]any) (out any, err error) {
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
}

// constClosure returns v itself: literals are boxed once at compile time.
func constClosure(v any) closure {
	return func(map[string]any) (any, error) { return v, nil }
}

// boolOperand evaluates c and asserts the result is a bool, which is what the
// VM's jump and not opcodes do. The transpiler wraps every logical operand in
// __bool, so the assertion only fails where the VM would have failed too.
func boolOperand(c closure, env map[string]any) (bool, error) {
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

func compileNode(node ast.Node, h *Helpers) (closure, bool) {
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
		return func(env map[string]any) (any, error) {
			return env[name], nil
		}, true

	case *ast.UnaryNode:
		if n.Operator != "!" && n.Operator != "not" {
			return nil, false
		}
		x, ok := compileNode(n.Node, h)
		if !ok {
			return nil, false
		}
		return func(env map[string]any) (any, error) {
			b, err := boolOperand(x, env)
			if err != nil {
				return nil, err
			}
			return !b, nil
		}, true

	case *ast.BinaryNode:
		l, ok := compileNode(n.Left, h)
		if !ok {
			return nil, false
		}
		r, ok := compileNode(n.Right, h)
		if !ok {
			return nil, false
		}
		switch n.Operator {
		case "&&", "and":
			return func(env map[string]any) (any, error) {
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
			return func(env map[string]any) (any, error) {
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
		c, ok := compileNode(n.Cond, h)
		if !ok {
			return nil, false
		}
		t, ok := compileNode(n.Exp1, h)
		if !ok {
			return nil, false
		}
		f, ok := compileNode(n.Exp2, h)
		if !ok {
			return nil, false
		}
		return func(env map[string]any) (any, error) {
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
		return compileCall(n, h)
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

func compileCall(n *ast.CallNode, h *Helpers) (closure, bool) {
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
			x, ok := compileNode(n.Arguments[0], h)
			if !ok {
				return nil, false
			}
			fn := h.Truthy
			return func(env map[string]any) (any, error) {
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
		l, ok := compileNode(n.Arguments[1], h)
		if !ok {
			return nil, false
		}
		r, ok := compileNode(n.Arguments[2], h)
		if !ok {
			return nil, false
		}
		switch name {
		case "__arith":
			fn := h.Arith
			return func(env map[string]any) (any, error) {
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
			fn := h.Compare
			return func(env map[string]any) (any, error) {
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
			fn := h.Bitwise
			return func(env map[string]any) (any, error) {
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
		l, ok := compileNode(n.Arguments[0], h)
		if !ok {
			return nil, false
		}
		r, ok := compileNode(n.Arguments[1], h)
		if !ok {
			return nil, false
		}
		switch name {
		case "__concat":
			fn := h.Concat
			return func(env map[string]any) (any, error) {
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
			fn := h.Index
			return func(env map[string]any) (any, error) {
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
			fn := h.InstanceOf
			return func(env map[string]any) (any, error) {
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
			fn := h.Pair
			return func(env map[string]any) (any, error) {
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
		x, ok := compileNode(n.Arguments[0], h)
		if !ok {
			return nil, false
		}
		fn := h.Negate
		if name == "__bitnot" {
			fn = h.BitNot
		}
		return func(env map[string]any) (any, error) {
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
		x, ok := compileNode(n.Arguments[1], h)
		if !ok {
			return nil, false
		}
		fn := h.Cast
		return func(env map[string]any) (any, error) {
			v, err := x(env)
			if err != nil {
				return nil, err
			}
			return fn(typ, v), nil
		}, true

	case "__array":
		items := make([]closure, len(n.Arguments))
		for i, a := range n.Arguments {
			c, ok := compileNode(a, h)
			if !ok {
				return nil, false
			}
			items[i] = c
		}
		fn := h.Array
		return func(env map[string]any) (any, error) {
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

	// Everything else — registered functions, the interpreter-re-entering
	// helpers, transpiled closures — is a per-runtime value in the eval env,
	// installed by Eval for exactly the names the expression calls. The
	// argument slice is inherent to their variadic signature; what the
	// closure path drops is the VM's own dispatch around it.
	args := make([]closure, len(n.Arguments))
	for i, a := range n.Arguments {
		c, ok := compileNode(a, h)
		if !ok {
			return nil, false
		}
		args[i] = c
	}
	return func(env map[string]any) (any, error) {
		fn, ok := env[name].(func(...any) (any, error))
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
