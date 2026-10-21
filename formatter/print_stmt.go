package formatter

import (
	"strings"

	"github.com/titpetric/phpscript/model"
)

func (p *printer) printStmt(s model.Stmt) {
	switch n := s.(type) {
	case *model.InlineHTML:
		if p.inPHP {
			p.blank()
			p.line("?>")
			p.inPHP = false
		}
		// Raw HTML outside PHP tags: emit verbatim without indent.
		p.buf.WriteString(n.Text)
	case *model.Echo:
		args := make([]string, len(n.Args))
		for i, a := range n.Args {
			args[i] = p.expr(a)
		}
		p.line("echo " + strings.Join(args, ", ") + ";")
	case *model.ExprStmt:
		p.line(p.expr(n.X) + ";")
	case *model.Assign:
		p.line(p.assign(n) + ";")
	case *model.If:
		p.printIf(n, false)
	case *model.Foreach:
		p.printForeach(n)
	case *model.For:
		p.printFor(n)
	case *model.DoWhile:
		p.line("do {")
		p.body(n.Body, p.stmtEnd(n))
		p.line("} while (" + p.expr(n.Cond) + ");")
	case *model.Return:
		if n.Value == nil {
			p.line("return;")
		} else {
			p.line("return " + p.expr(n.Value) + ";")
		}
	case *model.Include:
		kw := includeKeyword(n)
		if n.Parenthesized {
			p.line(kw + "(" + p.expr(n.Path) + ");")
		} else {
			p.line(kw + " " + p.expr(n.Path) + ";")
		}
	case *model.FuncDecl:
		p.printFunc(n, false)
	case *model.ClassDecl:
		p.printClass(n)
	case *model.InterfaceDecl:
		p.printInterface(n)
	case *model.Throw:
		p.line("throw " + p.expr(n.X) + ";")
	case *model.Try:
		p.printTry(n)
	case *model.Switch:
		p.printSwitch(n)
	case *model.Break:
		p.line("break;")
	case *model.Continue:
		p.line("continue;")
	case *model.Unset:
		p.line("unset(" + p.args(n.Targets) + ");")
	case *model.Global:
		p.line("global $" + strings.Join(n.Names, ", $") + ";")
	case *model.StaticVar:
		decls := make([]string, len(n.Vars))
		for i, d := range n.Vars {
			decls[i] = "$" + d.Name
			if d.Default != nil {
				decls[i] += " = " + p.expr(d.Default)
			}
		}
		p.line("static " + strings.Join(decls, ", ") + ";")
	case *model.ConstDecl:
		decls := make([]string, len(n.Consts))
		for i, c := range n.Consts {
			decls[i] = c.Name + " = " + p.expr(c.Default)
		}
		p.line("const " + strings.Join(decls, ", ") + ";")
	case *model.Use:
		p.line(p.use(n))
	case *model.Declare:
		p.printDeclare(n)
	default:
		p.line(p.unsupportedNode("statement", s))
	}
}

// use renders an import statement. The parser resolves imports while parsing,
// so the statement is inert, but dropping it would rewrite a file into one
// that states none of its dependencies.
func (p *printer) use(n *model.Use) string {
	out := "use "
	if n.Kind != "" {
		out += n.Kind + " "
	}
	names := make([]string, 0, len(n.Imports))
	for _, imp := range n.Imports {
		name := imp.Path
		if imp.Alias != "" {
			name += " as " + imp.Alias
		}
		names = append(names, name)
	}
	return out + strings.Join(names, ", ") + ";"
}

func (p *printer) printDeclare(n *model.Declare) {
	parts := make([]string, 0, len(n.Directives))
	for _, d := range n.Directives {
		parts = append(parts, d.Name+"="+p.expr(d.Value))
	}
	head := "declare(" + strings.Join(parts, ", ") + ")"
	if !n.Block {
		p.line(head + ";")
		return
	}
	p.line(head + " {")
	p.body(n.Body, p.stmtEnd(n))
	p.line("}")
}

func (p *printer) assign(n *model.Assign) string {
	if n.Op == "[]=" {
		return p.expr(n.Target) + "[] = " + p.expr(n.Value)
	}
	return p.expr(n.Target) + " " + n.Op + " " + p.expr(n.Value)
}

func (p *printer) printIf(n *model.If, elseif bool) {
	kw := "if"
	if elseif {
		kw = "elseif"
	}
	p.line(kw + " (" + p.expr(n.Cond) + ") {")
	p.body(n.Then, p.thenEnd(n))
	if len(n.Else) == 1 {
		if nested, ok := n.Else[0].(*model.If); ok {
			// Write "} elseif ..." without a blank close line.
			p.ensurePHP()
			p.buf.WriteString(p.indent() + "} ")
			// printIf for elseif writes the full line including indent, so trim.
			p.printElseIf(nested)
			return
		}
	}
	if len(n.Else) > 0 {
		p.line("} else {")
		p.body(n.Else, p.stmtEnd(n))
	}
	p.line("}")
}

// thenEnd is the line the `then` arm of an if closes on: the `else` keyword
// when there is one, and otherwise the closing brace of the statement.
func (p *printer) thenEnd(n *model.If) int {
	if n.ElseLine > 0 {
		return n.ElseLine
	}
	return p.stmtEnd(n)
}

func (p *printer) printElseIf(n *model.If) {
	// Continuation after "} " already written without newline finish.
	p.buf.WriteString("elseif (" + p.expr(n.Cond) + ") {\n")
	p.body(n.Then, p.thenEnd(n))
	if len(n.Else) == 1 {
		if nested, ok := n.Else[0].(*model.If); ok {
			p.ensurePHP()
			p.buf.WriteString(p.indent() + "} ")
			p.printElseIf(nested)
			return
		}
	}
	if len(n.Else) > 0 {
		p.line("} else {")
		p.body(n.Else, p.stmtEnd(n))
	}
	p.line("}")
}

func (p *printer) printForeach(n *model.Foreach) {
	val := p.expr(n.ValTarget)
	if n.ValTarget == nil && n.ValVar != "" {
		val = "$" + n.ValVar
	}
	head := "foreach (" + p.expr(n.Source) + " as "
	if n.KeyTarget != nil {
		head += p.expr(n.KeyTarget) + " => " + val
	} else if n.KeyVar != "" {
		head += "$" + n.KeyVar + " => " + val
	} else {
		head += val
	}
	head += ") {"
	p.line(head)
	p.body(n.Body, p.stmtEnd(n))
	p.line("}")
}

func (p *printer) printFor(n *model.For) {
	// A while loop arrives as a For with nil Init/Post.
	if n.Init == nil && n.Post == nil {
		p.line("while (" + p.expr(n.Cond) + ") {")
		p.body(n.Body, p.stmtEnd(n))
		p.line("}")
		return
	}
	init := p.simpleStmt(n.Init)
	post := p.simpleStmt(n.Post)
	cond := ""
	if n.Cond != nil {
		cond = p.expr(n.Cond)
	}
	p.line("for (" + init + "; " + cond + "; " + post + ") {")
	p.body(n.Body, p.stmtEnd(n))
	p.line("}")
}

func (p *printer) simpleStmt(s model.Stmt) string {
	if s == nil {
		return ""
	}
	switch n := s.(type) {
	case *model.Assign:
		return p.assign(n)
	case *model.ExprStmt:
		return p.expr(n.X)
	default:
		return ""
	}
}

// catchClause renders the `(...)` of a catch: a type filter, a bound variable,
// or both, in whichever combination the source used.
func catchClause(c model.Catch) string {
	parts := make([]string, 0, 2)
	if c.Type != "" {
		parts = append(parts, c.Type)
	}
	if c.Var != "" {
		parts = append(parts, "$"+c.Var)
	}
	return strings.Join(parts, " ")
}

// tryEnd is the line the try block closes on: the first `catch`, or `finally`
// when there is no catch clause.
func tryEnd(n *model.Try) int {
	if len(n.Catches) > 0 {
		return n.Catches[0].Line
	}
	return n.FinallyLine
}

func (p *printer) printTry(n *model.Try) {
	p.line("try {")
	p.body(n.Body, tryEnd(n))
	for i, c := range n.Catches {
		p.line("} catch (" + catchClause(c) + ") {")
		end := p.stmtEnd(n)
		if i+1 < len(n.Catches) {
			end = n.Catches[i+1].Line
		} else if n.FinallyLine > 0 {
			end = n.FinallyLine
		}
		p.body(c.Body, end)
	}
	if len(n.Finally) > 0 {
		p.line("} finally {")
		p.body(n.Finally, p.stmtEnd(n))
	}
	p.line("}")
}

func (p *printer) printSwitch(n *model.Switch) {
	p.line("switch (" + p.expr(n.Cond) + ") {")
	p.depth++
	for _, c := range n.Cases {
		if c.Line > 0 {
			p.flushComments(c.Line)
			p.blankFor(c.Line)
			p.lastLine = c.Line
		}
		p.line("case " + p.expr(c.Value) + ":")
		// A comment between two arms belongs to the arm below it, so the
		// bodies leave their trailing comments to the flush above.
		p.body(c.Body, 0)
	}
	if len(n.Default) > 0 {
		p.line("default:")
		p.body(n.Default, 0)
	}
	p.flushComments(p.stmtEnd(n))
	p.depth--
	p.line("}")
}

func isDecl(s model.Stmt) bool {
	switch s.(type) {
	case *model.ClassDecl, *model.InterfaceDecl, *model.FuncDecl:
		return true
	}
	return false
}

func isFuncDecl(s model.Stmt) bool {
	_, ok := s.(*model.FuncDecl)
	return ok
}

func isBlockStmt(s model.Stmt) bool {
	switch s.(type) {
	case *model.If, *model.Foreach, *model.For, *model.DoWhile, *model.Try, *model.Switch:
		return true
	}
	return false
}

func isCallStmt(s model.Stmt) bool {
	n, ok := s.(*model.ExprStmt)
	if !ok {
		return false
	}
	switch n.X.(type) {
	case *model.Call, *model.MethodCall:
		return true
	}
	return false
}
