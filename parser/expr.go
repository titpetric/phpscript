package parser

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// Expression parsing uses precedence climbing. The table below mirrors the
// subset of PHP operators the runner/transpiler understands.
//
// Note: the README forbids assignment inside conditions (no `if ($a = "b")`),
// so `=` is NOT an expression operator here; assignment is a statement only.
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
		return p.newAssignExpr(left, op, right, t.line), nil
	}
	return left, nil
}

// isLValue reports whether e can be assigned to.
func isLValue(e model.Expr) bool {
	switch model.UnwrapParenthesized(e).(type) {
	case *model.Var, *model.Index, *model.PropAccess, *model.StaticProp, *model.ListExpr:
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
		// PHP binds an assignment tighter than the comparison to its left, so
		// `false !== $pos = strrpos($s, "\\")` assigns and then compares. The
		// operand parser cannot see this (it stops at the operator), so the
		// assignment is folded onto the right operand here.
		if t := p.cur(); t.kind == tOp && isAssignOp(t.val) && isLValue(right) {
			p.next()
			value, err := p.parseAssign()
			if err != nil {
				return nil, err
			}
			right = p.newAssignExpr(right, t.val, value, t.line)
		}
		left = p.newBinary(op, left, right)
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
	if p.isOp("++") || p.isOp("--") {
		op := p.next().val
		x, err := p.parsePostfix()
		if err != nil {
			return nil, err
		}
		if !isLValue(x) {
			return nil, fmt.Errorf("line %d: %s requires assignable target", p.cur().line, op)
		}
		return p.newUnary(op, x, false), nil
	}
	if p.isOp("!") || p.isOp("-") || p.isOp("+") {
		op := p.next().val
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return p.newUnary(op, x, false), nil
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
				e = p.newMethodCall(e, name, args)
			} else {
				e = p.newProp(e, name)
			}
		case p.isOp("["):
			p.next()
			if p.isOp("]") {
				// `$a[]` append target, represented as Index with nil index.
				p.next()
				e = p.newIndex(e, nil)
				continue
			}
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.eatOp("]"); err != nil {
				return nil, err
			}
			e = p.newIndex(e, idx)
		case p.isOp("("):
			// Calling a value rather than a name: `$fn($x)`, `$handlers[0]($x)`,
			// `(self::$includeFile)($file)`. Named calls never reach here;
			// they are consumed by parsePrimary.
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			e = &model.Invoke{Callee: e, Args: args}
		case (p.isOp("++") || p.isOp("--")) && isLValue(e):
			e = p.newUnary(p.next().val, e, true)
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
		return p.newVar(t.val), nil

	case tInt, tFloat:
		p.next()
		v, err := numLit(t)
		if err != nil {
			return nil, err
		}
		return p.newLit(v), nil

	case tString:
		p.next()
		return p.newStringLit(t.val, t.quote), nil

	case tIdent:
		return p.parseIdentExpr()

	case tOp:
		switch t.val {
		case "\\":
			name, _, err := p.parseQualifiedName(true)
			if err != nil {
				return nil, err
			}
			return p.parseNamedExpr(name, true)
		case "(":
			p.next()
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.eatOp(")"); err != nil {
				return nil, err
			}
			return p.newParen(e), nil
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
	// `static function () {}` is a closure declared to have no `$this`. Any
	// other `static` is the late-static-binding class name and falls through to
	// the qualified-name path below (`static::method()`).
	if p.isKw("static") && p.peek(1).kind == tIdent && isFuncKeyword(p.peek(1).val) {
		p.next() // static
		p.next() // function
		cl, err := p.parseClosure()
		if err != nil {
			return nil, err
		}
		cl.Static = true
		return cl, nil
	}
	t := p.next()
	switch t.val {
	case "true", "TRUE", "True":
		return p.newLit(true), nil
	case "false", "FALSE", "False":
		return p.newLit(false), nil
	case "null", "NULL", "Null":
		return p.newLit(nil), nil
	case "new":
		return p.parseNew()
	case "fn", "func", "function":
		cl, err := p.parseClosure()
		if err != nil {
			return nil, err
		}
		return cl, nil
	case "list":
		return p.parseList()
	case "include", "include_once", "require", "require_once":
		return p.parseIncludeExpr(t.val)
	case "array":
		if p.isOp("(") {
			return p.parseArrayLiteral("(", ")")
		}
		return p.newLit(nil), nil
	}
	name := t.val
	for p.isOp("\\") {
		p.next()
		if p.cur().kind != tIdent {
			return nil, fmt.Errorf("line %d: expected name segment", p.cur().line)
		}
		name += "\\" + p.next().val
	}
	return p.parseNamedExpr(name, false)
}

func (p *parser) parseNamedExpr(name string, absolute bool) (model.Expr, error) {
	// `Class::` introduces a static member: a constant, a static property or a
	// static method call, told apart by what follows the operator.
	if p.isOp("::") {
		p.next()
		class := p.qualify(name, absolute)
		if p.cur().kind == tVar {
			return &model.StaticProp{Class: class, Name: p.next().val}, nil
		}
		if p.cur().kind != tIdent {
			return nil, fmt.Errorf("line %d: expected member name after ::", p.cur().line)
		}
		member := p.next().val
		if p.isOp("(") {
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			return &model.StaticCall{Class: class, Method: member, Args: args}, nil
		}
		return &model.ClassConst{Class: class, Name: member}, nil
	}

	// Function call vs. bare identifier (treated as constant lookup via a Call
	// with no args is wrong; use a zero-arg function only when parens present).
	if p.isOp("(") {
		args, err := p.parseArgs()
		if err != nil {
			return nil, err
		}
		if !absolute && p.namespace != "" && !strings.ContainsRune(name, '\\') {
			return p.newCall(p.qualify(name, false), name, args), nil
		}
		return p.newCall(p.qualify(name, absolute), "", args), nil
	}
	// Bare identifier: a constant. Model it as a Var so the env can resolve it,
	// or as a literal string fallback. We use Call-free Var-like lookup via a
	// no-arg marker is overkill; represent as a Var reference.
	if name == "__NAMESPACE__" {
		return p.newLit(p.namespace), nil
	}
	return p.newConstRef(name), nil
}

// isFuncKeyword reports whether name introduces a function body.
func isFuncKeyword(name string) bool {
	return name == "function" || name == "fn" || name == "func"
}

// parseClosure parses an anonymous function `function(params) [use(...)] { body }`.
// The `use` capture list names the enclosing variables the closure sees; the
// runtime binds their values when the closure is created.
func (p *parser) parseClosure() (*model.Closure, error) {
	params, err := p.parseParams()
	if err != nil {
		return nil, err
	}
	var uses []model.ClosureUse
	if p.isKw("use") {
		p.next()
		uses, err = p.parseClosureUses()
		if err != nil {
			return nil, err
		}
	}
	p.skipReturnType()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &model.Closure{Params: params, Uses: uses, Body: body}, nil
}

// parseClosureUses parses a closure's `use ($a, &$b)` capture list.
func (p *parser) parseClosureUses() ([]model.ClosureUse, error) {
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	var uses []model.ClosureUse
	for !p.isOp(")") {
		byRef := false
		if p.isOp("&") {
			p.next()
			byRef = true
		}
		if p.cur().kind != tVar {
			return nil, fmt.Errorf("line %d: expected captured $var in use()", p.cur().line)
		}
		uses = append(uses, model.ClosureUse{Name: p.next().val, ByRef: byRef})
		if p.isOp(",") {
			p.next()
		}
	}
	return uses, p.eatOp(")")
}

// parseList parses `list($a, $b, ...)` used as an assignment target. Empty
// slots (`list(, $b)`) are represented by nil elements.
func (p *parser) parseList() (model.Expr, error) {
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	mark := p.exprs.mark()
	for !p.isOp(")") {
		if p.isOp(",") {
			p.exprs.push(nil)
			p.next()
			continue
		}
		e, err := p.parseExpr()
		if err != nil {
			p.exprs.drop(mark)
			return nil, err
		}
		p.exprs.push(e)
		if p.isOp(",") {
			p.next()
		}
	}
	return &model.ListExpr{Elems: p.exprs.take(mark)}, p.eatOp(")")
}

func (p *parser) parseNew() (model.Expr, error) {
	class, absolute, err := p.parseQualifiedName(true)
	if err != nil {
		return nil, fmt.Errorf("line %d: expected class name after new: %w", p.cur().line, err)
	}
	n := &model.New{Class: p.qualify(class, absolute)}
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
	mark := p.exprs.mark()
	for !p.isOp(")") {
		e, err := p.parseExpr()
		if err != nil {
			p.exprs.drop(mark)
			return nil, err
		}
		p.exprs.push(e)
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	return p.exprs.take(mark), p.eatOp(")")
}

func (p *parser) parseArrayLiteral(open, close string) (model.Expr, error) {
	if err := p.eatOp(open); err != nil {
		return nil, err
	}
	mark := p.items.mark()
	for !p.isOp(close) {
		first, err := p.parseExpr()
		if err != nil {
			p.items.drop(mark)
			return nil, err
		}
		item := model.ArrayItem{Val: first}
		if p.isOp("=>") {
			p.next()
			val, err := p.parseExpr()
			if err != nil {
				p.items.drop(mark)
				return nil, err
			}
			item = model.ArrayItem{Key: first, Val: val}
		}
		p.items.push(item)
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	return p.newArrayLit(p.items.take(mark)), p.eatOp(close)
}
