package expr

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// This file holds the direct compiler's operator sub-compilers: an
// interpolated string, which is a compiled concatenation, the unary operators
// and the binary operators. The dispatcher that reaches them lives in
// direct.go.

// compileInterp compiles an interpolated string to one closure that writes
// every part into a single builder. This compiles the parts once and an
// embedded expression costs what the same expression costs anywhere else and
// the literal pays no dispatch of its own per execution.
//
// The builder is what a fold over Concat cannot do: a chain of pairwise
// concatenations allocates an intermediate string per part, where this
// allocates the result once. A lone literal part is itself, and the empty literal
// is the result for a literal with no parts at all.
func (dc *directCompiler) compileInterp(n *model.Interp) (closure, error) {
	if len(n.Parts) == 0 {
		return constClosure(""), nil
	}
	if len(n.Parts) == 1 {
		if lit, isLit := n.Parts[0].(*model.Lit); isLit {
			return constClosure(lit.Value), nil
		}
	}
	parts, err := dc.compileArgs(n.Parts)
	if err != nil {
		return nil, err
	}
	str := dc.h.Str
	return func(env *Env) (any, error) {
		var out strings.Builder
		for _, part := range parts {
			v, err := part(env)
			if err != nil {
				return nil, err
			}
			out.WriteString(str(v))
		}
		return out.String(), nil
	}, nil
}

func (dc *directCompiler) compileUnary(n *model.Unary) (closure, error) {
	switch n.Op {
	case "!", "~", "-", "+":
	case "++", "--":
		// The value comes from the live scope at evaluation time and not
		// from the slot snapshot, and the target is a general lvalue:
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
	// `instanceof` with a bare name on the right names the class, and no
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
