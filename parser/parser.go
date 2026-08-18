// Package parser turns PHP source (the limited subset described in the project
// README) into the shared model AST consumed by the runner.
//
// Scope of v0: the parser targets the README's "desired syntax" subset: php
// tags, $-variables, echo, assignment, if/elseif/else, foreach, for, while,
// function and class declarations, the `function Class::method()` form, method
// and property access via both `->` and `.`, array/new expressions, and the
// usual operators. It intentionally does not implement the full PHP grammar
// (see the README "omissions" list: inheritance, interfaces, traits, etc.).
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// Parse compiles PHP source into a model.Program.
func Parse(src string) (*model.Program, error) {
	toks, err := newLexer(src).run()
	if err != nil {
		return nil, err
	}
	// One span per statement; statements run around one per sixteen tokens, so
	// the hint saves the map's rehash-and-copy cycle (rule 6).
	p := &parser{toks: toks, spans: make(map[model.Stmt]model.SourceSpan, len(toks)/16+8)}
	stmts, err := p.parseStmts(true)
	if err != nil {
		return nil, err
	}
	return &model.Program{Stmts: stmts, Namespace: p.namespace, SourceSpans: p.spans}, nil
}

type parser struct {
	toks      []token
	i         int
	namespace string
	// imports maps the short name (or explicit alias) declared by a `use`
	// statement to the fully-qualified name it stands for. It stays nil in the
	// common case of a file with no imports.
	imports  map[string]string
	inClass  bool
	topLevel bool
	topSeen  bool
	spans    map[model.Stmt]model.SourceSpan

	// Chunked node storage and list scratch stacks; see alloc.go.
	varNodes        nodeChunk[model.Var]
	litNodes        nodeChunk[model.Lit]
	binaryNodes     nodeChunk[model.Binary]
	indexNodes      nodeChunk[model.Index]
	propNodes       nodeChunk[model.PropAccess]
	methodNodes     nodeChunk[model.MethodCall]
	callNodes       nodeChunk[model.Call]
	parenNodes      nodeChunk[model.Parenthesized]
	unaryNodes      nodeChunk[model.Unary]
	assignExprNodes nodeChunk[model.AssignExpr]
	exprStmtNodes   nodeChunk[model.ExprStmt]
	assignNodes     nodeChunk[model.Assign]
	echoNodes       nodeChunk[model.Echo]
	arrayLitNodes   nodeChunk[model.ArrayLit]

	exprs  scratch[model.Expr]
	stmts  scratch[model.Stmt]
	params scratch[model.Param]
	items  scratch[model.ArrayItem]
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

func (p *parser) isKw(vss ...string) bool {
	for _, v := range vss {
		t := p.cur()
		if t.kind == tIdent && t.val == v {
			return true
		}
	}
	return false
}

func (p *parser) eatOp(v string) error {
	if !p.isOp(v) {
		return fmt.Errorf("line %d: expected %q, got %s", p.cur().line, v, p.cur())
	}
	p.next()
	return nil
}

// parseStmts parses statements until EOF (top) or a closing brace.
func (p *parser) parseStmts(top bool) ([]model.Stmt, error) {
	wasTopLevel := p.topLevel
	p.topLevel = top
	defer func() { p.topLevel = wasTopLevel }()
	mark := p.stmts.mark()
	for !p.atEOF() {
		if !top && p.isOp("}") {
			break
		}
		s, err := p.parseStmt()
		if err != nil {
			p.stmts.drop(mark)
			return nil, err
		}
		if s != nil {
			if top {
				p.topSeen = true
			}
			if top && p.namespace != "" {
				switch s.(type) {
				case *model.ClassDecl, *model.FuncDecl:
				default:
					p.stmts.drop(mark)
					return nil, fmt.Errorf("line %d: namespaced files may only declare symbols", p.cur().line)
				}
			}
			p.stmts.push(s)
		}
	}
	return p.stmts.take(mark), nil
}

func (p *parser) parseStmt() (model.Stmt, error) {
	start := p.cur().line
	s, err := p.parseStmtNode()
	if err == nil && s != nil {
		end := start
		if p.i > 0 {
			end = p.toks[p.i-1].line
		}
		p.spans[s] = model.SourceSpan{Start: start, End: end}
	}
	return s, err
}

func (p *parser) parseStmtNode() (model.Stmt, error) {
	t := p.cur()

	if t.kind == tInlineHTML {
		p.next()
		return &model.InlineHTML{Text: t.val}, nil
	}

	if t.kind == tIdent {
		switch t.val {
		case "namespace":
			return p.parseNamespace()
		case "use":
			return p.parseUse()
		case "declare":
			return p.parseDeclare()
		case "unset":
			return p.parseUnset()
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
		case "die", "exit":
			if p.peek(1).kind != tOp || p.peek(1).val != "(" {
				p.next()
				p.optSemi()
				return &model.ExprStmt{X: &model.Call{Name: t.val, Bare: true}}, nil
			}
		case "fn", "func", "function":
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

func (p *parser) parseNamespace() (model.Stmt, error) {
	line := p.next().line
	if !p.topLevel {
		return nil, fmt.Errorf("line %d: namespace declarations are only allowed at the top level", line)
	}
	if p.topSeen {
		return nil, fmt.Errorf("line %d: namespace declaration must be the first statement", line)
	}
	name, _, err := p.parseQualifiedName(false)
	if err != nil {
		return nil, fmt.Errorf("line %d: invalid namespace: %w", line, err)
	}
	if name == "" {
		return nil, fmt.Errorf("line %d: expected namespace name", line)
	}
	if err := p.eatOp(";"); err != nil {
		return nil, err
	}
	p.namespace = name
	return nil, nil
}

// parseUse records an import: `use A\B\C;` or `use A\B\C as D;`. Imports are a
// compile-time alias table, where the short name (or the alias) resolves to
// the fully-qualified one, so the statement itself produces no AST node.
//
// `use function f;` and `use const C;` name symbols rather than classes; both
// alias the same way, so the leading keyword is simply consumed.
func (p *parser) parseUse() (model.Stmt, error) {
	p.next() // use
	if p.isKw("function") || p.isKw("const") {
		p.next()
	}
	for {
		name, _, err := p.parseQualifiedName(true)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid use: %w", p.cur().line, err)
		}
		alias := name
		if i := strings.LastIndexByte(alias, '\\'); i >= 0 {
			alias = alias[i+1:]
		}
		if p.isKw("as") {
			p.next()
			if p.cur().kind != tIdent {
				return nil, fmt.Errorf("line %d: expected alias name after as", p.cur().line)
			}
			alias = p.next().val
		}
		if p.imports == nil {
			p.imports = map[string]string{}
		}
		p.imports[alias] = name
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	p.optSemi()
	return nil, nil
}

// parseDeclare consumes a `declare(directive=value, ...)` block. The runtime
// has one set of semantics, and neither `strict_types` nor `ticks` varies it,
// so the directives are read and dropped rather than modelled.
func (p *parser) parseDeclare() (model.Stmt, error) {
	p.next() // declare
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	depth := 1
	for depth > 0 {
		if p.atEOF() {
			return nil, fmt.Errorf("line %d: unterminated declare()", p.cur().line)
		}
		switch {
		case p.isOp("("):
			depth++
		case p.isOp(")"):
			depth--
		}
		p.next()
	}
	// `declare(ticks=1) { ... }` scopes the directive to a block; the block is
	// ordinary code and still runs.
	if p.isOp("{") {
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		return &model.If{Cond: &model.Lit{Value: true}, Then: body}, nil
	}
	p.optSemi()
	return nil, nil
}

// parseUnset parses `unset($a, $b[$k], $o->p, C::$s)`.
func (p *parser) parseUnset() (model.Stmt, error) {
	p.next() // unset
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	var targets []model.Expr
	for !p.isOp(")") {
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		targets = append(targets, e)
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	if err := p.eatOp(")"); err != nil {
		return nil, err
	}
	p.optSemi()
	return &model.Unset{Targets: targets}, nil
}

// parseQualifiedName consumes Ident (\\ Ident)*. If leading is allowed, an
// initial namespace separator marks the name as fully qualified.
func (p *parser) parseQualifiedName(leading bool) (string, bool, error) {
	absolute := false
	if leading && p.isOp("\\") {
		absolute = true
		p.next()
	}
	if p.cur().kind != tIdent {
		return "", absolute, fmt.Errorf("expected name, got %s", p.cur())
	}
	first := p.next().val
	// Unqualified name (the common case): the token text is the whole name, so
	// neither the parts slice nor the Join copy is needed.
	if !p.isOp("\\") {
		return first, absolute, nil
	}
	parts := []string{first}
	for p.isOp("\\") {
		p.next()
		if p.cur().kind != tIdent {
			return "", absolute, fmt.Errorf("expected name segment, got %s", p.cur())
		}
		parts = append(parts, p.next().val)
	}
	return strings.Join(parts, "\\"), absolute, nil
}

func (p *parser) qualify(name string, absolute bool) string {
	if absolute || name == "self" || name == "static" || name == "parent" {
		return name
	}
	// An imported name wins over the current namespace: `use A\B\C;` makes a
	// later `C` mean `A\B\C`, and `C\D` mean `A\B\C\D`.
	if len(p.imports) > 0 {
		head, rest, _ := strings.Cut(name, "\\")
		if full, ok := p.imports[head]; ok {
			if rest == "" {
				return full
			}
			return full + "\\" + rest
		}
	}
	if p.namespace == "" {
		return name
	}
	return p.namespace + "\\" + name
}

func (p *parser) parseEcho() (model.Stmt, error) {
	p.next() // echo
	mark := p.exprs.mark()
	for {
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
	p.optSemi()
	return p.newEcho(p.exprs.take(mark)), nil
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
	wrapped := p.isOp("(")
	if wrapped {
		p.next()
	}
	src, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if !p.isKw("as") {
		return nil, fmt.Errorf("line %d: expected 'as' in foreach", p.cur().line)
	}
	p.next()
	first, firstByRef, err := p.parseForeachTarget()
	if err != nil {
		return nil, err
	}
	node := &model.Foreach{Source: src, ValTarget: first, ByRef: firstByRef}
	if v, ok := first.(*model.Var); ok {
		node.ValVar = v.Name
	}
	if p.isOp("=>") {
		p.next()
		val, valByRef, err := p.parseForeachTarget()
		if err != nil {
			return nil, err
		}
		// `&` on the key half is not PHP; only the value can bind by reference.
		if firstByRef {
			return nil, fmt.Errorf("line %d: a foreach key cannot be taken by reference", p.cur().line)
		}
		node.ByRef = valByRef
		node.KeyTarget = first
		node.ValTarget = val
		node.ValVar = ""
		if v, ok := first.(*model.Var); ok {
			node.KeyVar = v.Name
		}
		if v, ok := val.(*model.Var); ok {
			node.ValVar = v.Name
		}
	}
	if wrapped {
		if err := p.eatOp(")"); err != nil {
			return nil, err
		}
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node.Body = body
	return node, nil
}

// parseForeachTarget parses one `as` target and reports whether it was written
// `&$v`, which binds the element itself rather than a copy of it.
func (p *parser) parseForeachTarget() (model.Expr, bool, error) {
	byRef := false
	if p.isOp("&") {
		p.next()
		byRef = true
	}
	target, err := p.parsePostfix()
	if err != nil {
		return nil, false, err
	}
	if !isLValue(target) {
		return nil, false, fmt.Errorf("line %d: expected assignable target in foreach", p.cur().line)
	}
	return target, byRef, nil
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
	e, err := p.parseIncludeExpr(kw)
	if err != nil {
		return nil, err
	}
	p.optSemi()
	return e, nil
}

func (p *parser) parseIncludeExpr(kw string) (*model.Include, error) {
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
	return &model.Include{Path: e, Keyword: kw, Once: once, Parenthesized: hadParen}, nil
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
	mark := p.stmts.mark()
	for !p.isOp("}") && !p.atEOF() && !p.isKw("case") && !p.isKw("default") {
		s, err := p.parseStmt()
		if err != nil {
			p.stmts.drop(mark)
			return nil, err
		}
		if s != nil {
			p.stmts.push(s)
		}
	}
	return p.stmts.take(mark), nil
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
	} else if p.namespace != "" && !p.inClass {
		fd.Name = p.namespace + "\\" + fd.Name
	}

	params, err := p.parseParams()
	if err != nil {
		return nil, err
	}
	fd.Params = params
	p.skipReturnType()
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
	mark := p.params.mark()
	for !p.isOp(")") {
		p.skipParamDecoration()
		if p.cur().kind != tVar {
			p.params.drop(mark)
			return nil, fmt.Errorf("line %d: expected parameter $var", p.cur().line)
		}
		pr := model.Param{Name: p.next().val}
		if p.isOp("=") {
			p.next()
			def, err := p.parseExpr()
			if err != nil {
				p.params.drop(mark)
				return nil, err
			}
			pr.Default = def
		}
		p.params.push(pr)
		if p.isOp(",") {
			p.next()
		}
	}
	return p.params.take(mark), p.eatOp(")")
}

// skipParamDecoration consumes everything PHP allows between the opening
// parenthesis (or a comma) and a parameter's `$name`: constructor property
// promotion modifiers, a type hint, the by-reference `&`, and the variadic
// `...`. The runtime is dynamically typed and has no reference values, so none
// of it changes how the parameter binds; it only has to be recognised.
func (p *parser) skipParamDecoration() {
	for p.isKw("public", "private", "protected", "readonly") {
		p.next()
	}
	p.skipTypeHint()
	if p.isOp("&") {
		p.next()
	}
	for p.isOp(".") {
		p.next()
	}
}

// skipTypeHint consumes an optional type: `?Name`, `\Ns\Name`, `array`, and
// `A|B` unions. It stops at the first token that cannot continue a type, so a
// parameter list with no hints costs one comparison.
func (p *parser) skipTypeHint() {
	for {
		if p.isOp("?") {
			p.next()
			continue
		}
		if p.isOp("\\") {
			p.next()
			continue
		}
		if p.cur().kind != tIdent {
			return
		}
		p.next()
		for p.isOp("\\") {
			p.next()
			if p.cur().kind == tIdent {
				p.next()
			}
		}
		if p.isOp("|") {
			p.next()
			continue
		}
		return
	}
}

// skipReturnType consumes a `: Type` return declaration, which sits between a
// parameter list and its body.
func (p *parser) skipReturnType() {
	if !p.isOp(":") {
		return
	}
	p.next()
	p.skipTypeHint()
}

func (p *parser) parseClass(abstract bool) (model.Stmt, error) {
	p.next() // class
	if p.cur().kind != tIdent {
		return nil, fmt.Errorf("line %d: expected class name", p.cur().line)
	}
	cd := &model.ClassDecl{Name: p.qualify(p.next().val, false), Abstract: abstract}
	wasInClass := p.inClass
	p.inClass = true
	defer func() { p.inClass = wasInClass }()
	if err := p.eatOp("{"); err != nil {
		return nil, err
	}
	for !p.isOp("}") && !p.atEOF() {
		memberStart := p.cur().line
		// Collect leading modifiers. Visibility/static are recorded for
		// formatting; abstract marks a body-less method.
		visibility := ""
		isStatic := false
		methodAbstract := false
		for p.isKw("public") || p.isKw("private") || p.isKw("protected") ||
			p.isKw("static") || p.isKw("final") || p.isKw("abstract") {
			switch {
			case p.isKw("public"), p.isKw("private"), p.isKw("protected"):
				visibility = p.cur().val
			case p.isKw("static"):
				isStatic = true
			case p.isKw("abstract"):
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
			for i := range consts {
				consts[i].Visibility = visibility
			}
			cd.Consts = append(cd.Consts, consts...)
		case p.isKw("var"):
			p.next()
			fields, err := p.parseFields(visibility)
			if err != nil {
				return nil, err
			}
			cd.Fields = append(cd.Fields, fields...)
		case p.isKw("fn", "func", "function"):
			if methodAbstract {
				m, err := p.parseAbstractMethod(visibility, isStatic)
				if err != nil {
					return nil, err
				}
				p.spans[m] = model.SourceSpan{Start: memberStart, End: p.toks[p.i-1].line}
				cd.Methods = append(cd.Methods, m)
				continue
			}
			m, err := p.parseFunction()
			if err != nil {
				return nil, err
			}
			fd := m.(*model.FuncDecl)
			fd.Visibility = visibility
			fd.Static = isStatic
			p.spans[fd] = model.SourceSpan{Start: memberStart, End: p.toks[p.i-1].line}
			cd.Methods = append(cd.Methods, fd)
		case p.cur().kind == tVar || p.cur().kind == tIdent || p.isOp("?"):
			// Property declared without `var`, with or without a type hint
			// (`protected $stack = array();`, `private ?string $dir = null;`).
			p.skipTypeHint()
			if p.cur().kind != tVar {
				return nil, fmt.Errorf("line %d: unexpected token in class body: %s", p.cur().line, p.cur())
			}
			fields, err := p.parseFields(visibility)
			if err != nil {
				return nil, err
			}
			// A static property is storage on the class, shared by every
			// instance, so it is kept apart from the per-instance fields.
			if isStatic {
				cd.Statics = append(cd.Statics, fields...)
				continue
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

// parseAbstractMethod consumes `function name(params);` with no body.
func (p *parser) parseAbstractMethod(visibility string, isStatic bool) (*model.FuncDecl, error) {
	p.next() // function
	if p.cur().kind != tIdent {
		return nil, fmt.Errorf("line %d: expected method name", p.cur().line)
	}
	name := p.next().val
	params, err := p.parseParams()
	if err != nil {
		return nil, err
	}
	p.skipReturnType()
	p.optSemi()
	return &model.FuncDecl{
		Name:       name,
		Params:     params,
		Visibility: visibility,
		Static:     isStatic,
		Abstract:   true,
	}, nil
}

func (p *parser) parseFields(visibility string) ([]model.Field, error) {
	var fields []model.Field
	for {
		if p.cur().kind != tVar {
			return nil, fmt.Errorf("line %d: expected $field", p.cur().line)
		}
		f := model.Field{Name: p.next().val, Visibility: visibility}
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
	if ae, ok := model.UnwrapParenthesized(lhs).(*model.AssignExpr); ok {
		return p.newAssign(ae.Target, ae.Op, ae.Value), nil
	}
	return p.newExprStmt(lhs), nil
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
