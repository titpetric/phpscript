package parser

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
)

// Expression parsing uses precedence climbing. The table below mirrors the
// subset of PHP operators the runner/transpiler understands.
//
// Note: the README forbids assignment inside conditions (no `if ($a = "b")`),
// so `=` is NOT an expression operator here — assignment is a statement only.
var binPrec = map[string]int{
	"||": 1, "&&": 2,
	"==": 3, "!=": 3, "===": 3, "!==": 3,
	"<": 4, "<=": 4, ">": 4, ">=": 4,
	".": 5,
	"+": 6, "-": 6,
	"*": 7, "/": 7, "%": 7,
}

func (p *parser) parseExpr() (model.Expr, error) {
	return p.parseAssign()
}

// parseAssign supports assignment used as an expression (e.g. the PHP idiom
// `if (($x = f()) !== false)`). It is right-associative and only valid when the
// left side is an lvalue (variable, index, property or list()).
func (p *parser) parseAssign() (model.Expr, error) {
	left, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if t := p.cur(); t.kind == tOp && isAssignOp(t.val) && isLValue(left) {
		op := p.next().val
		right, err := p.parseAssign()
		if err != nil {
			return nil, err
		}
		return &model.AssignExpr{Target: left, Op: op, Value: right, Line: t.line}, nil
	}
	return left, nil
}

// isLValue reports whether e can be assigned to.
func isLValue(e model.Expr) bool {
	switch e.(type) {
	case *model.Var, *model.Index, *model.PropAccess, *model.ListExpr:
		return true
	}
	return false
}

func (p *parser) parseTernary() (model.Expr, error) {
	cond, err := p.parseBinary(1)
	if err != nil {
		return nil, err
	}
	if p.isOp("?") {
		p.next()
		then := cond
		if !p.isOp(":") {
			then, err = p.parseExpr()
			if err != nil {
				return nil, err
			}
		}
		if err := p.eatOp(":"); err != nil {
			return nil, err
		}
		els, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return &model.Ternary{Cond: cond, Then: then, Else: els}, nil
	}
	return cond, nil
}

func (p *parser) parseBinary(minPrec int) (model.Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.cur()
		if t.kind != tOp {
			break
		}
		prec, ok := binPrec[t.val]
		if !ok || prec < minPrec {
			break
		}
		op := p.next().val
		right, err := p.parseBinary(prec + 1)
		if err != nil {
			return nil, err
		}
		left = &model.Binary{Op: op, Left: left, Right: right}
	}
	return left, nil
}

// castTypes are the recognised `(type)` cast keywords.
var castTypes = map[string]bool{
	"bool": true, "boolean": true, "int": true, "integer": true,
	"float": true, "double": true, "real": true, "string": true,
	"array": true, "object": true,
}

func (p *parser) parseUnary() (model.Expr, error) {
	// Type cast: `(type) expr`.
	if p.isOp("(") && p.peek(1).kind == tIdent && castTypes[p.peek(1).val] &&
		p.peek(2).kind == tOp && p.peek(2).val == ")" {
		p.next() // (
		typ := p.next().val
		p.next() // )
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &model.Cast{Type: typ, X: x}, nil
	}
	if p.isOp("!") || p.isOp("-") || p.isOp("+") {
		op := p.next().val
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &model.Unary{Op: op, X: x}, nil
	}
	// `@expr` error suppression and `&expr` reference are parse-level no-ops
	// (the VM has no by-reference values; callable arrays carry the marker
	// harmlessly).
	if p.isOp("@") || p.isOp("&") {
		p.next()
		return p.parseUnary()
	}
	return p.parsePostfix()
}

// parsePostfix parses a primary followed by any chain of ->method(), ->prop,
// .method(), .prop, [index] and (call) suffixes.
func (p *parser) parsePostfix() (model.Expr, error) {
	e, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.isOp("->"):
			// Member/method access. The README floats `.` notation for fields,
			// but `.` is also PHP string concatenation (used throughout the
			// minitpl T1 target). Those are irreconcilable without type info, so
			// v0 uses `->` for members and keeps `.` as concat.
			p.next()
			if p.cur().kind != tIdent {
				return nil, fmt.Errorf("line %d: expected member name", p.cur().line)
			}
			name := p.next().val
			if p.isOp("(") {
				args, err := p.parseArgs()
				if err != nil {
					return nil, err
				}
				e = &model.MethodCall{Base: e, Method: name, Args: args}
			} else {
				e = &model.PropAccess{Base: e, Name: name}
			}
		case p.isOp("["):
			p.next()
			if p.isOp("]") {
				// `$a[]` append target — represented as Index with nil index.
				p.next()
				e = &model.Index{Base: e, Index: nil}
				continue
			}
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.eatOp("]"); err != nil {
				return nil, err
			}
			e = &model.Index{Base: e, Index: idx}
		default:
			return e, nil
		}
	}
}

func (p *parser) parsePrimary() (model.Expr, error) {
	t := p.cur()
	switch t.kind {
	case tVar:
		p.next()
		return &model.Var{Name: t.val}, nil

	case tInt, tFloat:
		p.next()
		v, err := numLit(t)
		if err != nil {
			return nil, err
		}
		return &model.Lit{Value: v}, nil

	case tString:
		p.next()
		return &model.Lit{Value: t.val}, nil

	case tIdent:
		return p.parseIdentExpr()

	case tOp:
		switch t.val {
		case "(":
			p.next()
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			return e, p.eatOp(")")
		case "[":
			return p.parseArrayLiteral("[", "]")
		case "{":
			return p.parseArrayLiteral("{", "}")
		}
	}
	return nil, fmt.Errorf("line %d: unexpected token %s", t.line, t)
}

// parseIdentExpr handles keyword literals, `new`, `array(...)`, and bare
// function calls / constants.
func (p *parser) parseIdentExpr() (model.Expr, error) {
	t := p.next()
	switch t.val {
	case "true", "TRUE", "True":
		return &model.Lit{Value: true}, nil
	case "false", "FALSE", "False":
		return &model.Lit{Value: false}, nil
	case "null", "NULL", "Null":
		return &model.Lit{Value: nil}, nil
	case "new":
		return p.parseNew()
	case "function":
		return p.parseClosure()
	case "list":
		return p.parseList()
	case "array":
		if p.isOp("(") {
			return p.parseArrayLiteral("(", ")")
		}
		return &model.Lit{Value: nil}, nil
	}

	// `Class::CONST` / `self::CONST` class-constant access.
	if p.isOp("::") {
		p.next()
		if p.cur().kind != tIdent {
			return nil, fmt.Errorf("line %d: expected constant name after ::", p.cur().line)
		}
		return &model.ClassConst{Class: t.val, Name: p.next().val}, nil
	}

	// Function call vs. bare identifier (treated as constant lookup via a Call
	// with no args is wrong; use a zero-arg function only when parens present).
	if p.isOp("(") {
		args, err := p.parseArgs()
		if err != nil {
			return nil, err
		}
		return &model.Call{Name: t.val, Args: args}, nil
	}
	// Bare identifier: a constant. Model it as a Var so the env can resolve it,
	// or as a literal string fallback. We use Call-free Var-like lookup via a
	// no-arg marker is overkill; represent as a Var reference.
	return &model.Var{Name: t.val}, nil
}

// parseClosure parses an anonymous function `function(params) [use(...)] { body }`.
// The `use` capture list, if present, is consumed but ignored.
func (p *parser) parseClosure() (model.Expr, error) {
	params, err := p.parseParams()
	if err != nil {
		return nil, err
	}
	if p.isKw("use") {
		p.next()
		// consume the use(...) list; captures are not modelled.
		if _, err := p.parseParams(); err != nil {
			return nil, err
		}
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &model.Closure{Params: params, Body: body}, nil
}

// parseList parses `list($a, $b, ...)` used as an assignment target. Empty
// slots (`list(, $b)`) are represented by nil elements.
func (p *parser) parseList() (model.Expr, error) {
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	lst := &model.ListExpr{}
	for !p.isOp(")") {
		if p.isOp(",") {
			lst.Elems = append(lst.Elems, nil)
			p.next()
			continue
		}
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		lst.Elems = append(lst.Elems, e)
		if p.isOp(",") {
			p.next()
		}
	}
	return lst, p.eatOp(")")
}

func (p *parser) parseNew() (model.Expr, error) {
	// Tolerate a leading namespace separator (e.g. `new \Exception`).
	for p.isOp("\\") {
		p.next()
	}
	if p.cur().kind != tIdent {
		return nil, fmt.Errorf("line %d: expected class name after new", p.cur().line)
	}
	class := p.next().val
	// Tolerate namespaced names `A\B\C` — keep only the final segment.
	for p.isOp("\\") {
		p.next()
		if p.cur().kind != tIdent {
			return nil, fmt.Errorf("line %d: expected class name segment", p.cur().line)
		}
		class = p.next().val
	}
	n := &model.New{Class: class}
	if p.isOp("(") {
		args, err := p.parseArgs()
		if err != nil {
			return nil, err
		}
		n.Args = args
	}
	return n, nil
}

func (p *parser) parseArgs() ([]model.Expr, error) {
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	var args []model.Expr
	for !p.isOp(")") {
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, e)
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	return args, p.eatOp(")")
}

func (p *parser) parseArrayLiteral(open, close string) (model.Expr, error) {
	if err := p.eatOp(open); err != nil {
		return nil, err
	}
	lit := &model.ArrayLit{}
	for !p.isOp(close) {
		first, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		item := model.ArrayItem{Val: first}
		if p.isOp("=>") {
			p.next()
			val, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			item = model.ArrayItem{Key: first, Val: val}
		}
		lit.Items = append(lit.Items, item)
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	return lit, p.eatOp(close)
}
