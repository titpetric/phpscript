// Package parser turns PHP source (the limited subset described in the project
// README) into the shared model AST consumed by the runner.
//
// Scope of v0: the parser targets the README's "desired syntax" subset: php
// tags, $-variables, echo, assignment, if/elseif/else, foreach, for, while,
// function and class declarations, the `function Class::method()` form, method
// and property access via both `->` and `.`, array/new expressions, and the
// usual operators. It intentionally does not implement the full PHP grammar
// (see the README "omissions" list: inheritance, traits, etc.). `extends` on a
// class is an exception in one direction only: it is parsed and recorded on the
// AST so files carrying it lint and reformat, but nothing downstream inherits.
// `interface` declarations and a class's `implements` list are parsed and
// checked as a contract, which also inherits nothing: see model.CheckInterfaces.
package parser

import (
	"errors"
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
	return &model.Program{Stmts: stmts, Namespace: p.namespace, NamespaceLine: p.namespaceLine, SourceSpans: p.spans}, nil
}

type parser struct {
	toks      []token
	i         int
	namespace string
	// imports maps the short name (or explicit alias) declared by a `use`
	// statement to the fully-qualified name it stands for. It stays nil in the
	// common case of a file with no imports.
	imports       map[string]string
	inClass       bool
	topLevel      bool
	topSeen       bool
	spans         map[model.Stmt]model.SourceSpan
	namespaceLine int

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
			if top && !isPreambleStmt(s) {
				p.topSeen = true
			}
			// An included namespaced file is scanned for the symbols it
			// declares rather than executed, which is what makes resolving a
			// name cheap. The restriction is a policy, not an omission; it is
			// recorded under "Known divergences from PHP" in docs/README.md,
			// so the message says why rather than only what.
			if top && p.namespace != "" && !isPreambleStmt(s) {
				switch s.(type) {
				case *model.ClassDecl, *model.InterfaceDecl, *model.FuncDecl:
				default:
					p.stmts.drop(mark)
					return nil, fmt.Errorf("line %d: a namespaced file may only declare classes and functions, "+
						"because it is scanned for the symbols it declares at include time instead of being run; "+
						"move this statement into a function, or into a file that declares no namespace", p.spans[s].Start)
				}
			}
			p.stmts.push(s)
		}
	}
	return p.stmts.take(mark), nil
}

// isPreambleStmt reports whether s is a file-preamble statement: an import or
// a directive rather than code. Both are retained in the AST so the formatter
// can print them back, and neither counts as the "first statement" that closes
// the window for a `namespace` declaration, nor as code in a namespaced file.
func isPreambleStmt(s model.Stmt) bool {
	switch n := s.(type) {
	case *model.Use:
		return true
	case *model.Declare:
		return !n.Block
	}
	return false
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
		case "abstract", "final", "readonly":
			// `readonly` is not a reserved word in PHP, so `readonly(...)` has
			// to keep parsing as a call; it is a modifier only when the run it
			// starts ends in `class`. `abstract` and `final` are reserved, so
			// anything else after them is a parse error worth reporting.
			if t.val == "readonly" && !p.classFollowsModifiers() {
				break
			}
			return p.parseModifiedClass()
		case "class":
			return p.parseClass(classModifiers{})
		case "interface":
			return p.parseInterface()
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
			return &model.Break{Line: t.line}, nil
		case "continue":
			p.next()
			p.optSemi()
			return &model.Continue{Line: t.line}, nil
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
	p.namespaceLine = line
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
	kind := ""
	if p.isKw("function") || p.isKw("const") {
		kind = p.cur().val
		p.next()
	}
	var imports []model.UseImport
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
		imports = append(imports, model.UseImport{Path: name, Alias: aliasSpelling(name, alias)})
		if p.isOp(",") {
			p.next()
			continue
		}
		break
	}
	p.optSemi()
	return &model.Use{Kind: kind, Imports: imports}, nil
}

// aliasSpelling returns the alias to print for an import, which is empty when
// the alias is just the short name of the path (`use A\B;` binds `B` with no
// `as` clause in the source).
func aliasSpelling(path, alias string) string {
	if i := strings.LastIndexByte(path, '\\'); i >= 0 {
		path = path[i+1:]
	}
	if path == alias {
		return ""
	}
	return alias
}

// parseDeclare parses `declare(directive=value, ...)`. The runtime has one set
// of semantics, and neither `strict_types` nor `ticks` varies it, so the
// directives are recorded for printing and otherwise ignored.
func (p *parser) parseDeclare() (model.Stmt, error) {
	p.next() // declare
	if err := p.eatOp("("); err != nil {
		return nil, err
	}
	out := &model.Declare{}
	for !p.isOp(")") {
		if p.atEOF() {
			return nil, fmt.Errorf("line %d: unterminated declare()", p.cur().line)
		}
		if p.cur().kind != tIdent {
			return nil, fmt.Errorf("line %d: expected declare directive, got %s", p.cur().line, p.cur())
		}
		name := p.next().val
		if err := p.eatOp("="); err != nil {
			return nil, err
		}
		value, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		out.Directives = append(out.Directives, model.DeclareDirective{Name: name, Value: value})
		if p.isOp(",") {
			p.next()
		}
	}
	if err := p.eatOp(")"); err != nil {
		return nil, err
	}
	// `declare(ticks=1) { ... }` scopes the directive to a block; the block is
	// ordinary code and still runs.
	if p.isOp("{") {
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		out.Body, out.Block = body, true
		return out, nil
	}
	p.optSemi()
	return out, nil
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
		node.ElseLine = p.cur().line
		nested, err := p.parseIf()
		if err != nil {
			return nil, err
		}
		node.Else = []model.Stmt{nested}
	case p.isKw("else"):
		node.ElseLine = p.cur().line
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
// The VM has no exception class hierarchy, so catch type filters are recorded
// for printing but not matched; the first catch clause handles any error.
func (p *parser) parseTry() (model.Stmt, error) {
	p.next() // try
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	tr := &model.Try{Body: body}
	for p.isKw("catch") {
		catchLine := p.cur().line
		p.next() // catch
		if err := p.eatOp("("); err != nil {
			return nil, err
		}
		// Exception type filter: `Type`, `A|B` or a qualified `\Ns\Type`.
		// The runtime ignores it, but PHP requires it, so it is recorded.
		catchType := p.parseTypeHint()
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
		tr.Catches = append(tr.Catches, model.Catch{Type: catchType, Var: name, Body: cbody, Line: catchLine})
	}
	if p.isKw("finally") {
		tr.FinallyLine = p.cur().line
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
			caseLine := p.cur().line
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
			sw.Cases = append(sw.Cases, model.SwitchCase{Value: val, Body: body, Line: caseLine})
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
	fd.ReturnType = p.parseReturnType()
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
		var pr model.Param
		p.parseParamDecoration(&pr)
		if p.cur().kind != tVar {
			p.params.drop(mark)
			return nil, fmt.Errorf("line %d: expected parameter $var", p.cur().line)
		}
		pr.Name = p.next().val
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
// parseParamDecoration consumes what may precede a parameter name: the
// visibility and readonly modifiers of a promoted constructor property, a type
// hint, `&` for a reference and `...` for a variadic. All of it is recorded on
// the parameter so the formatter can print the declaration back unchanged.
func (p *parser) parseParamDecoration(pr *model.Param) {
	var mods []string
	for p.isKw("public", "private", "protected", "readonly") {
		mods = append(mods, p.cur().val)
		p.next()
	}
	pr.Modifiers = strings.Join(mods, " ")
	pr.Type = p.parseTypeHint()
	if p.isOp("&") {
		p.next()
		pr.ByRef = true
	}
	for p.isOp(".") {
		p.next()
		pr.Variadic = true
	}
}

// parseTypeHint consumes an optional type: `?Name`, `\Ns\Name`, `array`, and
// `A|B` unions, and returns its spelling. It stops at the first token that
// cannot continue a type, so a parameter list with no hints costs one
// comparison.
//
// The runtime ignores types, but the formatter rewrites files in place, so a
// type it cannot see is a type it deletes.
func (p *parser) parseTypeHint() string {
	var b strings.Builder
	take := func() {
		b.WriteString(p.cur().val)
		p.next()
	}
	for {
		if p.isOp("?") {
			take()
			continue
		}
		if p.isOp("\\") {
			take()
			continue
		}
		if p.cur().kind != tIdent {
			return b.String()
		}
		take()
		for p.isOp("\\") {
			take()
			if p.cur().kind == tIdent {
				take()
			}
		}
		if p.isOp("|") {
			take()
			continue
		}
		return b.String()
	}
}

// parseReturnType consumes a `: Type` return declaration, which sits between a
// parameter list and its body, and returns the type spelling.
func (p *parser) parseReturnType() string {
	if !p.isOp(":") {
		return ""
	}
	p.next()
	return p.parseTypeHint()
}

// classModifiers are the keywords that may precede a `class` declaration. PHP
// accepts them in any order, so they are collected before the class keyword is
// reached rather than matched as a fixed prefix.
type classModifiers struct {
	abstract bool
	final    bool
	readonly bool
}

// classFollowsModifiers reports whether the run of modifier keywords starting
// at the cursor terminates in `class`.
func (p *parser) classFollowsModifiers() bool {
	for n := 0; ; n++ {
		t := p.peek(n)
		if t.kind != tIdent {
			return false
		}
		switch t.val {
		case "abstract", "final", "readonly":
			continue
		case "class":
			return true
		}
		return false
	}
}

// parseModifiedClass consumes `abstract`/`final`/`readonly` in any order and
// the `class` declaration they apply to. The modifiers are recorded on the AST
// so the formatter can print them back; none of them is enforced at runtime.
func (p *parser) parseModifiedClass() (model.Stmt, error) {
	var mods classModifiers
	line := p.cur().line
	for p.isKw("abstract", "final", "readonly") {
		kw := p.next().val
		seen := &mods.abstract
		switch kw {
		case "final":
			seen = &mods.final
		case "readonly":
			seen = &mods.readonly
		}
		if *seen {
			return nil, fmt.Errorf("line %d: multiple %s modifiers are not allowed", line, kw)
		}
		*seen = true
	}
	// `abstract final` is a contradiction: nothing could ever instantiate the
	// class. PHP rejects it at compile time and so does this.
	if mods.abstract && mods.final {
		return nil, fmt.Errorf("line %d: cannot use the final modifier on an abstract class", line)
	}
	if !p.isKw("class") {
		return nil, fmt.Errorf("line %d: expected class after class modifiers, got %s", p.cur().line, p.cur())
	}
	return p.parseClass(mods)
}

// parseClassHeritage consumes `extends Name` and `implements A, B` and records
// the names on cd. Recording is all it does: the runtime has no inheritance, so
// a parent contributes no members and an interface is not checked. The names
// are kept because the formatter rewrites files in place and would otherwise
// drop them.
func (p *parser) parseClassHeritage(cd *model.ClassDecl) error {
	if p.isKw("extends") {
		p.next()
		name, absolute, err := p.parseQualifiedName(true)
		if err != nil {
			return fmt.Errorf("line %d: expected class name after extends: %w", p.cur().line, err)
		}
		cd.Parent = p.qualify(name, absolute)
	}
	if !p.isKw("implements") {
		return nil
	}
	p.next()
	for {
		name, absolute, err := p.parseQualifiedName(true)
		if err != nil {
			return fmt.Errorf("line %d: expected interface name after implements: %w", p.cur().line, err)
		}
		cd.Implements = append(cd.Implements, p.qualify(name, absolute))
		if !p.isOp(",") {
			return nil
		}
		p.next()
	}
}

func (p *parser) parseClass(mods classModifiers) (model.Stmt, error) {
	p.next() // class
	if p.cur().kind != tIdent {
		return nil, fmt.Errorf("line %d: expected class name", p.cur().line)
	}
	cd := &model.ClassDecl{
		Name:     p.qualify(p.next().val, false),
		Abstract: mods.abstract,
		Final:    mods.final,
		Readonly: mods.readonly,
	}
	if err := p.parseClassHeritage(cd); err != nil {
		return nil, err
	}
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
				consts[i].Span = model.SourceSpan{Start: memberStart, End: p.toks[p.i-1].line}
			}
			cd.Consts = append(cd.Consts, consts...)
		case p.isKw("var"):
			p.next()
			fields, err := p.parseFields(visibility)
			if err != nil {
				return nil, err
			}
			p.setFieldSpans(fields, memberStart)
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
			fieldType := p.parseTypeHint()
			if p.cur().kind != tVar {
				return nil, fmt.Errorf("line %d: unexpected token in class body: %s", p.cur().line, p.cur())
			}
			fields, err := p.parseFields(visibility)
			if err != nil {
				return nil, err
			}
			p.setFieldSpans(fields, memberStart)
			for i := range fields {
				fields[i].Type = fieldType
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

// parseInterface consumes `interface Name [extends A, B] { ... }`.
//
// The body holds method signatures with no body, class constants, and the
// `static` spelling of a signature. It holds no property, because an interface
// declares no storage, and no method body, because it declares no behaviour:
// what it declares is a list of names a class saying `implements` must declare
// itself. See docs/design.md.
func (p *parser) parseInterface() (model.Stmt, error) {
	p.next() // interface
	if p.cur().kind != tIdent {
		return nil, fmt.Errorf("line %d: expected interface name", p.cur().line)
	}
	id := &model.InterfaceDecl{Name: p.qualify(p.next().val, false)}
	if p.isKw("extends") {
		p.next()
		for {
			name, absolute, err := p.parseQualifiedName(true)
			if err != nil {
				return nil, fmt.Errorf("line %d: expected interface name after extends: %w", p.cur().line, err)
			}
			id.Extends = append(id.Extends, p.qualify(name, absolute))
			if !p.isOp(",") {
				break
			}
			p.next()
		}
	}
	if err := p.eatOp("{"); err != nil {
		return nil, err
	}
	for !p.isOp("}") && !p.atEOF() {
		memberStart := p.cur().line
		visibility := ""
		isStatic := false
		// PHP accepts only `public` on an interface member, and rejects
		// `abstract` outright, but the modifiers are recorded rather than
		// enforced: what is written is what has to print back.
		for p.isKw("public", "private", "protected", "static", "final") {
			switch {
			case p.isKw("public"), p.isKw("private"), p.isKw("protected"):
				visibility = p.cur().val
			case p.isKw("static"):
				isStatic = true
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
				consts[i].Span = model.SourceSpan{Start: memberStart, End: p.toks[p.i-1].line}
			}
			id.Consts = append(id.Consts, consts...)
		case p.isKw("fn", "func", "function"):
			m, err := p.parseSignature(visibility, isStatic)
			if err != nil {
				return nil, err
			}
			p.spans[m] = model.SourceSpan{Start: memberStart, End: p.toks[p.i-1].line}
			id.Methods = append(id.Methods, m)
		default:
			return nil, fmt.Errorf("line %d: unexpected token in interface body: %s", p.cur().line, p.cur())
		}
	}
	return id, p.eatOp("}")
}

// parseSignature consumes `function name(params): type;`, a declaration with
// no body. It is what an interface member is; an abstract method in a class
// carries the same shape and adds the keyword.
func (p *parser) parseSignature(visibility string, isStatic bool) (*model.FuncDecl, error) {
	p.next() // function
	if p.cur().kind != tIdent {
		return nil, fmt.Errorf("line %d: expected method name", p.cur().line)
	}
	name := p.next().val
	params, err := p.parseParams()
	if err != nil {
		return nil, err
	}
	returnType := p.parseReturnType()
	p.optSemi()
	return &model.FuncDecl{
		Name:       name,
		Params:     params,
		ReturnType: returnType,
		Visibility: visibility,
		Static:     isStatic,
	}, nil
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
	fd, err := p.parseSignature(visibility, isStatic)
	if err != nil {
		return nil, err
	}
	fd.Abstract = true
	return fd, nil
}

// setFieldSpans records the source lines a property declaration occupies. One
// declaration can define several properties (`var $a, $b;`), which share it.
func (p *parser) setFieldSpans(fields []model.Field, start int) {
	span := model.SourceSpan{Start: start, End: p.toks[p.i-1].line}
	for i := range fields {
		fields[i].Span = span
	}
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
	case "=", ".=", "+=", "-=", "*=", "/=", "%=", "**=", "[]=",
		"&=", "|=", "^=", "<<=", ">>=":
		return true
	}
	return false
}

func (p *parser) optSemi() {
	if p.isOp(";") {
		p.next()
	}
}

// numLit parses an int/float token into a literal value. The underscore digit
// separator is not part of the value, and base 0 covers the prefixes PHP and
// Go spell the same way: 0x, 0b, 0o, and a leading zero for octal.
func numLit(t token) (any, error) {
	val := t.val
	if strings.ContainsRune(val, '_') {
		val = strings.ReplaceAll(val, "_", "")
	}
	if t.kind == tFloat {
		f, err := strconv.ParseFloat(val, 64)
		return f, err
	}
	i, err := strconv.ParseInt(val, 0, 64)
	if errors.Is(err, strconv.ErrRange) {
		// PHP widens an integer literal too large for an int to a float
		// instead of rejecting the program.
		if f, ferr := strconv.ParseFloat(val, 64); ferr == nil {
			return f, nil
		}
	}
	return i, err
}
