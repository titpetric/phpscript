// Package parser turns PHP source (the limited subset described in the project
// README) into the shared model AST consumed by the runner.
//
// Scope of v0: the parser targets the README's "desired syntax" subset — php
// tags, $-variables, echo, assignment, if/elseif/else, foreach, for, while,
// function and class declarations, the `function Class::method()` form, method
// and property access via both `->` and `.`, array/new expressions, and the
// usual operators. It intentionally does not implement the full PHP grammar
// (see the README "omissions" list: namespaces, inheritance, interfaces, etc.).
package parser

import (
	"fmt"
	"strconv"

	"github.com/titpetric/phpscript/model"
)

// Parse compiles PHP source into a model.Program.
func Parse(src string) (*model.Program, error) {
	toks, err := newLexer(src).run()
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	stmts, err := p.parseStmts(true)
	if err != nil {
		return nil, err
	}
	return &model.Program{Stmts: stmts}, nil
}

type parser struct {
	toks []token
	i    int
}

func (p *parser) cur() token { return p.toks[p.i] }

// peek returns the token n positions ahead, clamped to EOF.
func (p *parser) peek(n int) token {
	j := p.i + n
	if j >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}
	return p.toks[j]
}
func (p *parser) next() token {
	t := p.toks[p.i]
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return t
}

func (p *parser) atEOF() bool { return p.cur().kind == tEOF }

func (p *parser) isOp(v string) bool { t := p.cur(); return t.kind == tOp && t.val == v }
func (p *parser) isKw(v string) bool { t := p.cur(); return t.kind == tIdent && t.val == v }

func (p *parser) eatOp(v string) error {
	if !p.isOp(v) {
		return fmt.Errorf("line %d: expected %q, got %s", p.cur().line, v, p.cur())
	}
	p.next()
	return nil
}

// parseStmts parses statements until EOF (top) or a closing brace.
func (p *parser) parseStmts(top bool) ([]model.Stmt, error) {
	var out []model.Stmt
	for !p.atEOF() {
		if !top && p.isOp("}") {
			break
		}
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if s != nil {
			out = append(out, s)
		}
	}
	return out, nil
}

func (p *parser) parseStmt() (model.Stmt, error) {
	t := p.cur()

	if t.kind == tInlineHTML {
		p.next()
		return &model.InlineHTML{Text: t.val}, nil
	}

	if t.kind == tIdent {
		switch t.val {
		case "echo":
			return p.parseEcho()
		case "if":
			return p.parseIf()
		case "foreach":
			return p.parseForeach()
		case "for":
			return p.parseFor()
		case "while":
			return p.parseWhile()
		case "return":
			return p.parseReturn()
		case "function":
			// A bare `function(` is an anonymous closure expression statement,
			// not a declaration.
			if p.peek(1).kind == tOp && p.peek(1).val == "(" {
				break
			}
			return p.parseFunction()
		case "abstract":
			p.next() // abstract modifier
			if p.isKw("class") {
				return p.parseClass(true)
			}
			return nil, fmt.Errorf("line %d: expected class after abstract", p.cur().line)
		case "class":
			return p.parseClass(false)
		case "include", "include_once", "require", "require_once":
			return p.parseInclude()
		case "throw":
			return p.parseThrow()
		case "try":
			return p.parseTry()
		case "switch":
			return p.parseSwitch()
		case "break":
			p.next()
			p.optSemi()
			return &model.Break{}, nil
		case "continue":
			p.next()
			p.optSemi()
			return &model.Continue{}, nil
		case "var":
			// stray top-level `var` is unusual; treat like an expr stmt fallthrough
		}
	}

	// Bare `;` is an empty statement.
	if p.isOp(";") {
		p.next()
		return nil, nil
	}

	return p.parseExprStmt()
}

func (p *parser) parseEcho() (model.Stmt, error) {
	p.next() // echo
	var args []model.Expr
	for {
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
	p.optSemi()
	return &model.Echo{Args: args}, nil
}

func (p *parser) parseIf() (model.Stmt, error) {
	p.next() // if
	wrapped := p.isOp("(")
	if wrapped {
		p.next()
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if wrapped {
		if err := p.eatOp(")"); err != nil {
			return nil, err
		}
	}
	then, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node := &model.If{Cond: cond, Then: then}

	switch {
	case p.isKw("elseif"):
		// chain as a nested if in the else branch
		nested, err := p.parseIf()
		if err != nil {
			return nil, err
		}
		node.Else = []model.Stmt{nested}
	case p.isKw("else"):
		p.next()
		if p.isKw("if") {
			nested, err := p.parseIf()
			if err != nil {
				return nil, err
			}
			node.Else = []model.Stmt{nested}
		} else {
			els, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			node.Else = els
		}
	}
	return node, nil
}
func (p *parser) parseForeach() (model.Stmt, error) {
	p.next() // foreach
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	src, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if !p.isKw("as") {
		return nil, fmt.Errorf("line %d: expected 'as' in foreach", p.cur().line)
	}
	p.next()
	first, err := p.parseForeachVar()
	if err != nil {
		return nil, err
	}
	node := &model.Foreach{Source: src, ValVar: first}
	if p.isOp("=>") {
		p.next()
		val, err := p.parseForeachVar()
		if err != nil {
			return nil, err
		}
		node.KeyVar = first
		node.ValVar = val
	}
	if err := p.eatOp(")"); err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node.Body = body
	return node, nil
}

func (p *parser) parseForeachVar() (string, error) {
	if p.cur().kind != tVar {
		return "", fmt.Errorf("line %d: expected $var in foreach", p.cur().line)
	}
	return p.next().val, nil
}

func (p *parser) parseFor() (model.Stmt, error) {
	p.next() // for
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	init, err := p.parseSimpleStmt()
	if err != nil {
		return nil, err
	}
	if err := p.eatOp(";"); err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.eatOp(";"); err != nil {
		return nil, err
	}
	post, err := p.parseSimpleStmt()
	if err != nil {
		return nil, err
	}
	if err := p.eatOp(")"); err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &model.For{Init: init, Cond: cond, Post: post, Body: body}, nil
}

func (p *parser) parseWhile() (model.Stmt, error) {
	p.next() // while (alias to for with nil init/post)
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.eatOp(")"); err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &model.For{Cond: cond, Body: body}, nil
}

func (p *parser) parseReturn() (model.Stmt, error) {
	p.next() // return
	if p.isOp(";") {
		p.next()
		return &model.Return{}, nil
	}
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.optSemi()
	return &model.Return{Value: e}, nil
}

func (p *parser) parseInclude() (model.Stmt, error) {
	kw := p.next().val
	once := kw == "include_once" || kw == "require_once"
	hadParen := p.isOp("(")
	if hadParen {
		p.next()
	}
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if hadParen {
		if err := p.eatOp(")"); err != nil {
			return nil, err
		}
	}
	p.optSemi()
	return &model.Include{Path: e, Once: once}, nil
}

func (p *parser) parseThrow() (model.Stmt, error) {
	p.next() // throw
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.optSemi()
	return &model.Throw{X: e}, nil
}

// parseTry parses `try { ... } catch (Type $var) { ... } [finally { ... }]`.
// The VM has no exception class hierarchy, so catch type filters are consumed
// but ignored; the first catch clause handles any error.
func (p *parser) parseTry() (model.Stmt, error) {
	p.next() // try
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	tr := &model.Try{Body: body}
	for p.isKw("catch") {
		p.next() // catch
		if err := p.eatOp("("); err != nil {
			return nil, err
		}
		// Optional exception type filter(s): `Type` or `Type1 | Type2`.
		for p.cur().kind == tIdent {
			p.next()
			if p.isOp("|") {
				p.next()
			}
		}
		// Optional bound variable: `catch (Exception $e)` or PHP8 `catch (Exception)`.
		var name string
		if p.cur().kind == tVar {
			name = p.next().val
		}
		if err := p.eatOp(")"); err != nil {
			return nil, err
		}
		cbody, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		tr.Catches = append(tr.Catches, model.Catch{Var: name, Body: cbody})
	}
	if p.isKw("finally") {
		p.next()
		fbody, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		tr.Finally = fbody
	}
	return tr, nil
}

func (p *parser) parseSwitch() (model.Stmt, error) {
	p.next() // switch
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.eatOp(")"); err != nil {
		return nil, err
	}
	if err := p.eatOp("{"); err != nil {
		return nil, err
	}
	sw := &model.Switch{Cond: cond}
	for !p.isOp("}") && !p.atEOF() {
		switch {
		case p.isKw("case"):
			p.next()
			val, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if p.isOp(":") {
				p.next()
			} else if err := p.eatOp(";"); err != nil {
				return nil, err
			}
			body, err := p.parseCaseBody()
			if err != nil {
				return nil, err
			}
			sw.Cases = append(sw.Cases, model.SwitchCase{Value: val, Body: body})
		case p.isKw("default"):
			p.next()
			if p.isOp(":") {
				p.next()
			} else if err := p.eatOp(";"); err != nil {
				return nil, err
			}
			body, err := p.parseCaseBody()
			if err != nil {
				return nil, err
			}
			sw.Default = body
		default:
			return nil, fmt.Errorf("line %d: unexpected token in switch: %s", p.cur().line, p.cur())
		}
	}
	return sw, p.eatOp("}")
}

// parseCaseBody parses statements until the next case/default or closing brace.
func (p *parser) parseCaseBody() ([]model.Stmt, error) {
	var out []model.Stmt
	for !p.isOp("}") && !p.atEOF() && !p.isKw("case") && !p.isKw("default") {
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if s != nil {
			out = append(out, s)
		}
	}
	return out, nil
}

func (p *parser) parseFunction() (model.Stmt, error) {
	p.next() // function
	if p.cur().kind != tIdent {
		return nil, fmt.Errorf("line %d: expected function name", p.cur().line)
	}
	name := p.next().val
	fd := &model.FuncDecl{Name: name}

	// `function Class::method()` form.
	if p.isOp("::") {
		p.next()
		if p.cur().kind != tIdent {
			return nil, fmt.Errorf("line %d: expected method name after ::", p.cur().line)
		}
		fd.Class = name
		fd.Name = p.next().val
	}

	params, err := p.parseParams()
	if err != nil {
		return nil, err
	}
	fd.Params = params
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	fd.Body = body
	return fd, nil
}

func (p *parser) parseParams() ([]model.Param, error) {
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	var params []model.Param
	for !p.isOp(")") {
		if p.cur().kind != tVar {
			return nil, fmt.Errorf("line %d: expected parameter $var", p.cur().line)
		}
		pr := model.Param{Name: p.next().val}
		if p.isOp("=") {
			p.next()
			def, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			pr.Default = def
		}
		params = append(params, pr)
		if p.isOp(",") {
			p.next()
		}
	}
	return params, p.eatOp(")")
}

func (p *parser) parseClass(abstract bool) (model.Stmt, error) {
	p.next() // class
	if p.cur().kind != tIdent {
		return nil, fmt.Errorf("line %d: expected class name", p.cur().line)
	}
	cd := &model.ClassDecl{Name: p.next().val, Abstract: abstract}
	if err := p.eatOp("{"); err != nil {
		return nil, err
	}
	for !p.isOp("}") && !p.atEOF() {
		// Skip leading modifiers; visibility and static/abstract/final are
		// tolerated but not enforced (README omits method visibility). `abstract`
		// marks a body-less method.
		methodAbstract := false
		for p.isKw("public") || p.isKw("private") || p.isKw("protected") ||
			p.isKw("static") || p.isKw("final") || p.isKw("abstract") {
			if p.isKw("abstract") {
				methodAbstract = true
			}
			p.next()
		}

		switch {
		case p.isKw("const"):
			consts, err := p.parseConsts()
			if err != nil {
				return nil, err
			}
			cd.Consts = append(cd.Consts, consts...)
		case p.isKw("var"):
			p.next()
			fields, err := p.parseFields()
			if err != nil {
				return nil, err
			}
			cd.Fields = append(cd.Fields, fields...)
		case p.isKw("function"):
			if methodAbstract {
				// `abstract function name($args);` — declaration only, no body.
				if err := p.skipAbstractMethod(); err != nil {
					return nil, err
				}
				continue
			}
			m, err := p.parseFunction()
			if err != nil {
				return nil, err
			}
			cd.Methods = append(cd.Methods, m.(*model.FuncDecl))
		case p.cur().kind == tVar:
			// Typed/visibility-prefixed property without `var` (e.g.
			// `protected $stack = array();`).
			fields, err := p.parseFields()
			if err != nil {
				return nil, err
			}
			cd.Fields = append(cd.Fields, fields...)
		default:
			return nil, fmt.Errorf("line %d: unexpected token in class body: %s", p.cur().line, p.cur())
		}
	}
	return cd, p.eatOp("}")
}

// parseConsts parses `const NAME = expr [, NAME = expr];`.
func (p *parser) parseConsts() ([]model.Field, error) {
	p.next() // const
	var consts []model.Field
	for {
		if p.cur().kind != tIdent {
			return nil, fmt.Errorf("line %d: expected constant name", p.cur().line)
		}
		c := model.Field{Name: p.next().val}
		if err := p.eatOp("="); err != nil {
			return nil, err
		}
		v, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		c.Default = v
		consts = append(consts, c)
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	p.optSemi()
	return consts, nil
}

// skipAbstractMethod consumes `function name(params);` with no body.
func (p *parser) skipAbstractMethod() error {
	p.next() // function
	if p.cur().kind != tIdent {
		return fmt.Errorf("line %d: expected method name", p.cur().line)
	}
	p.next() // name
	if _, err := p.parseParams(); err != nil {
		return err
	}
	p.optSemi()
	return nil
}

func (p *parser) parseFields() ([]model.Field, error) {
	var fields []model.Field
	for {
		if p.cur().kind != tVar {
			return nil, fmt.Errorf("line %d: expected $field", p.cur().line)
		}
		f := model.Field{Name: p.next().val}
		if p.isOp("=") {
			p.next()
			def, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			f.Default = def
		}
		fields = append(fields, f)
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	p.optSemi()
	return fields, nil
}

func (p *parser) parseBlock() ([]model.Stmt, error) {
	// Brace-less single-statement body (e.g. `if (...) return;`).
	if !p.isOp("{") {
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if s == nil {
			return nil, nil
		}
		return []model.Stmt{s}, nil
	}
	if err := p.eatOp("{"); err != nil {
		return nil, err
	}
	stmts, err := p.parseStmts(false)
	if err != nil {
		return nil, err
	}
	return stmts, p.eatOp("}")
}

// parseExprStmt parses an expression / assignment terminated by an optional ';'.
func (p *parser) parseExprStmt() (model.Stmt, error) {
	s, err := p.parseSimpleStmt()
	if err != nil {
		return nil, err
	}
	p.optSemi()
	return s, nil
}

// parseSimpleStmt parses an assignment or expression statement WITHOUT consuming
// a terminator (used in for(;;) clauses too). May return nil for empty.
func (p *parser) parseSimpleStmt() (model.Stmt, error) {
	if p.isOp(";") || p.isOp(")") {
		return nil, nil
	}
	lhs, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	// parseExpr already folds assignment into an AssignExpr; lower it to a
	// statement-level Assign so the interpreter's mutation path handles it.
	if ae, ok := lhs.(*model.AssignExpr); ok {
		return &model.Assign{Target: ae.Target, Op: ae.Op, Value: ae.Value}, nil
	}
	return &model.ExprStmt{X: lhs}, nil
}

func isAssignOp(v string) bool {
	switch v {
	case "=", ".=", "+=", "-=", "*=", "/=", "%=", "[]=":
		return true
	}
	return false
}

func (p *parser) optSemi() {
	if p.isOp(";") {
		p.next()
	}
}

// numLit parses an int/float token into a literal value.
func numLit(t token) (any, error) {
	if t.kind == tFloat {
		f, err := strconv.ParseFloat(t.val, 64)
		return f, err
	}
	i, err := strconv.ParseInt(t.val, 10, 64)
	return i, err
}
