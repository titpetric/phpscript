// Package formatter pretty-prints phpscript ASTs in a gofmt-like style:
// hard tabs, canonical `function` keywords, parentheses on control structures,
// semicolons, collapsed blank lines, and a trailing newline. Class, function,
// and control-structure opening braces stay on the declaration line.
package formatter

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
)

// Source parses src and pretty-prints the AST.
func Source(src string) (string, error) {
	// An executable script starts with an interpreter line, which the lexer
	// skips. It is put back on the formatted output rather than costing the
	// script its formatting.
	if shebang, rest, ok := splitShebang(src); ok {
		out, err := Source(rest)
		if err != nil {
			return "", err
		}
		return shebang + "\n" + out, nil
	}
	// Files that start outside PHP are templates. Formatting their PHP blocks
	// independently is not supported yet, so leave the entire file unchanged.
	if !strings.HasPrefix(src, "<?php") {
		return src, nil
	}
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	out, unsupported := printProgram(prog, Options{Comments: CollectComments(src)})
	// Formatting rewrites the file in place, so a node the printer cannot
	// spell has to stop the rewrite: emitting the placeholder would delete
	// working code.
	if len(unsupported) > 0 {
		return "", fmt.Errorf("cannot format: unsupported %s", strings.Join(unsupported, ", "))
	}
	out = strings.ReplaceAll(out, "\r\n", "\n")
	return strings.ReplaceAll(out, "\r", "\n"), nil
}

// splitShebang separates a leading `#!` interpreter line from the rest of the
// file. It reports false for a file that does not have one.
func splitShebang(src string) (shebang, rest string, ok bool) {
	if !strings.HasPrefix(src, "#!") {
		return "", src, false
	}
	line, rest, found := strings.Cut(src, "\n")
	if !found {
		return "", src, false
	}
	return strings.TrimRight(line, "\r"), rest, true
}

// Options controls AST pretty-printing.
type Options struct {
	// Comments is every comment of the source file, in source order. The AST
	// holds none of them, so the printer places them by line: a comment is
	// written out before the first statement that starts below it.
	Comments []Comment
}

// Print renders prog as formatted PHP source. Any node the printer has no
// spelling for is rendered as a placeholder comment; use Source, which refuses
// to rewrite a file in that case.
func Print(prog *model.Program, opts Options) string {
	out, _ := printProgram(prog, opts)
	return out
}

// printProgram renders prog and reports the nodes it could not render.
func printProgram(prog *model.Program, opts Options) (string, []string) {
	startsInPHP := prog.Namespace != "" || len(opts.Comments) > 0 || len(prog.Stmts) == 0
	if len(prog.Stmts) > 0 {
		_, startsWithHTML := prog.Stmts[0].(*model.InlineHTML)
		startsInPHP = startsInPHP || !startsWithHTML
	}
	stmts := prog.Stmts
	if len(stmts) > 0 {
		if html, ok := stmts[len(stmts)-1].(*model.InlineHTML); ok && strings.TrimSpace(html.Text) == "" {
			stmts = stmts[:len(stmts)-1]
		}
	}
	p := &printer{depth: 0, namespace: prog.Namespace, inPHP: startsInPHP, spans: prog.SourceSpans, comments: opts.Comments}
	if startsInPHP {
		p.line("<?php")
		p.blank()
	}
	// `declare` has to come before the namespace declaration, which is printed
	// from Program.Namespace rather than from the statement list.
	directives, stmts := splitDeclarePreamble(stmts)
	if len(directives) > 0 {
		p.stmts(directives)
		p.blank()
	}
	if prog.Namespace != "" {
		p.flushComments(prog.NamespaceLine)
		p.blankFor(prog.NamespaceLine)
		p.line("namespace " + prog.Namespace + ";")
		p.lastLine = prog.NamespaceLine
		p.blank()
	}
	p.stmts(stmts)
	// Anything written below the last statement, a trailing note or a
	// commented-out block, still belongs to the file.
	p.flushComments(0)
	out := collapseBlankLines(p.buf.String())
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, p.unsupported
}

// splitDeclarePreamble separates the leading `declare(...);` statements from
// the rest of the program. PHP requires them to be the first statement in the
// file, ahead of the namespace declaration.
func splitDeclarePreamble(stmts []model.Stmt) (preamble, rest []model.Stmt) {
	for i, s := range stmts {
		d, ok := s.(*model.Declare)
		if !ok || d.Block {
			return stmts[:i], stmts[i:]
		}
	}
	return stmts, nil
}

type printer struct {
	buf       bytes.Buffer
	depth     int
	namespace string
	inPHP     bool
	spans     map[model.Stmt]model.SourceSpan
	// unsupported records the AST nodes the printer has no spelling for. The
	// formatter rewrites files in place, so printing a placeholder comment for
	// one would delete working code; Source reports them as an error instead
	// and leaves the file alone.
	unsupported []string
	// comments is the source comment stream, consumed in order: nextComment is
	// the first one not yet written out. lastLine is the source line of the
	// last thing written, which is what tells a blank line the author left
	// from one the printer would be inventing.
	comments    []Comment
	nextComment int
	lastLine    int
	// afterComment records that a comment was the last thing written, so a
	// declaration can follow its documentation without a blank line between.
	afterComment bool
}

// unsupportedNode records a node the printer cannot render and returns the
// placeholder text used in the (discarded) output.
func (p *printer) unsupportedNode(kind string, node any) string {
	text := fmt.Sprintf("/* unsupported %s %T */", kind, node)
	p.unsupported = append(p.unsupported, fmt.Sprintf("%s %T", kind, node))
	return text
}

func (p *printer) indent() string {
	return strings.Repeat("\t", p.depth)
}

func (p *printer) ensurePHP() {
	if p.inPHP {
		return
	}
	p.buf.WriteString("<?php\n")
	p.inPHP = true
}

func (p *printer) line(s string) {
	p.ensurePHP()
	p.buf.WriteString(p.indent())
	p.buf.WriteString(strings.TrimRight(s, " \t"))
	p.buf.WriteByte('\n')
}

func (p *printer) blank() {
	if p.inPHP {
		p.buf.WriteByte('\n')
	}
}

// stmts prints a statement list without leading or trailing blank lines. It
// retains one source blank line and adds separation after block statements and
// before a call that follows assignments.
func (p *printer) stmts(stmts []model.Stmt) {
	// A block opens with no blank line of its own, whatever the source did
	// between the brace and the first statement.
	p.lastLine = 0
	for i, s := range stmts {
		if i > 0 && p.blankBetween(stmts[i-1], s) {
			p.blank()
		}
		p.stmt(s)
	}
}

// body prints the statements of a block one level in, then writes out any
// comment left between the last statement and end, the line the block closes
// on. An end of zero leaves those comments for the enclosing block.
func (p *printer) body(stmts []model.Stmt, end int) {
	p.depth++
	p.stmts(stmts)
	if end > 0 {
		p.flushComments(end)
	}
	p.depth--
}

// stmtEnd is the line a statement closes on, used to place the comments
// written inside it.
func (p *printer) stmtEnd(s model.Stmt) int {
	return p.spans[s].End
}

// blankBetween reports the blank lines the printer adds of its own accord.
// The ones the author wrote are kept by stmt, which compares source lines.
func (p *printer) blankBetween(prev, next model.Stmt) bool {
	switch prev.(type) {
	case *model.ClassDecl, *model.InterfaceDecl:
		return false
	}
	if isBlockStmt(prev) {
		return true
	}
	if isDecl(next) || isFuncDecl(prev) {
		return true
	}
	_, assigned := prev.(*model.Assign)
	return assigned && isCallStmt(next)
}

func (p *printer) stmt(s model.Stmt) {
	if _, ok := s.(*model.InlineHTML); !ok {
		p.ensurePHP()
	}
	span, placed := p.spans[s]
	if placed {
		p.flushComments(span.Start)
		// A declaration follows its documentation without a gap; anything
		// else keeps the blank line the author left above it.
		if !p.afterComment || !isDecl(s) {
			p.blankFor(span.Start)
		}
	}
	p.printStmt(s)
	p.afterComment = false
	if placed {
		p.trailingComment(span.End)
		p.lastLine = span.End
	}
}

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
// that no longer states its dependencies.
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
	// while is represented as For with nil Init/Post.
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

func (p *printer) printFunc(n *model.FuncDecl, inClass bool) {
	var b strings.Builder
	if n.Visibility != "" {
		b.WriteString(n.Visibility)
		b.WriteByte(' ')
	}
	if n.Static {
		b.WriteString("static ")
	}
	if n.Abstract {
		b.WriteString("abstract ")
	}
	b.WriteString("function ")
	if n.ByRef {
		b.WriteString("&")
	}
	if n.Class != "" && !inClass {
		b.WriteString(shortName(n.Class))
		b.WriteString("::")
	}
	b.WriteString(shortName(n.Name))
	b.WriteString("(")
	b.WriteString(p.params(n.Params))
	b.WriteString(")")
	b.WriteString(returnType(n.ReturnType))
	if n.Abstract {
		b.WriteString(";")
		p.line(b.String())
		return
	}
	b.WriteString(" {")
	p.line(b.String())
	p.body(n.Body, p.stmtEnd(n))
	p.line("}")
}

// Class members are grouped: constants, then properties, then methods. The
// order is what a reader of an unfamiliar class needs first, and it is the
// order PHP style guides settle on.
// groupSignature is the interface counterpart of groupMethod: a body-less
// declaration is one line, so consecutive ones are kept together the way
// properties are, rather than separated by a blank line each.
const (
	groupConst = iota
	groupProp
	groupSignature
	groupMethod
)

// classMember is one declaration to print, carrying the group it belongs to
// and the source lines it occupied, which decide the blank lines around it.
type classMember struct {
	group int
	span  model.SourceSpan
	print func()
}

func (p *printer) printClass(n *model.ClassDecl) {
	// Modifiers are prepended in reverse of their printed order, which puts
	// them back in PHP's canonical `abstract|final readonly class` spelling
	// whatever order the source used.
	head := "class " + shortName(n.Name)
	if n.Readonly {
		head = "readonly " + head
	}
	if n.Final {
		head = "final " + head
	}
	if n.Abstract {
		head = "abstract " + head
	}
	if n.Parent != "" {
		head += " extends " + p.typeName(n.Parent)
	}
	for i, name := range n.Implements {
		if i == 0 {
			head += " implements "
		} else {
			head += ", "
		}
		head += p.typeName(name)
	}
	p.line(head + " {")
	p.depth++
	members := make([]classMember, 0, len(n.Consts)+len(n.Fields)+len(n.Statics)+len(n.Methods))
	for _, c := range n.Consts {
		members = append(members, classMember{group: groupConst, span: c.Span, print: func() {
			vis := ""
			if c.Visibility != "" {
				vis = c.Visibility + " "
			}
			p.line(vis + "const " + c.Name + " = " + p.expr(c.Default) + ";")
		}})
	}
	for _, f := range n.Fields {
		members = append(members, classMember{group: groupProp, span: f.Span, print: func() {
			p.line(p.field(f) + ";")
		}})
	}
	for _, f := range n.Statics {
		members = append(members, classMember{group: groupProp, span: f.Span, print: func() {
			p.line(p.staticField(f) + ";")
		}})
	}
	for _, m := range n.Methods {
		members = append(members, classMember{group: groupMethod, span: p.spans[m], print: func() {
			p.printFunc(m, true)
		}})
	}
	// Properties keep the order they were declared in; the parser collects
	// static ones separately because they are different storage.
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].group != members[j].group {
			return members[i].group < members[j].group
		}
		return members[i].span.Start < members[j].span.Start
	})
	p.printMembers(members, p.stmtEnd(n))
	p.depth--
	p.line("}")
}

// printMembers writes the members of a class or an interface body, separating
// the groups with a blank line and keeping the ones the author left between
// members of the same group. end is the line the body closes on, which is where
// the comments written after the last member belong.
func (p *printer) printMembers(members []classMember, end int) {
	prev := classMember{group: -1}
	p.lastLine = 0
	for _, m := range members {
		p.flushComments(m.span.Start)
		switch {
		case prev.group == -1:
		case m.group != prev.group, m.group == groupMethod:
			if !p.afterComment {
				p.blank()
			}
		case m.span.Start > prev.span.End+1:
			// A blank line the author left between two groups of properties
			// or constants is theirs to keep.
			if !p.afterComment {
				p.blank()
			}
		}
		m.print()
		p.afterComment = false
		p.trailingComment(m.span.End)
		p.lastLine = m.span.End
		prev = m
	}
	p.flushComments(end)
}

// printInterface renders an `interface Name extends A, B { ... }` declaration:
// its constants, then its method signatures, each closed with a semicolon
// because an interface method has no body. PHP rejects `abstract` on one, so
// the keyword is never printed here even though the shared FuncDecl can carry
// it for an abstract class method.
func (p *printer) printInterface(n *model.InterfaceDecl) {
	head := "interface " + shortName(n.Name)
	for i, name := range n.Extends {
		if i == 0 {
			head += " extends "
		} else {
			head += ", "
		}
		head += p.typeName(name)
	}
	p.line(head + " {")
	p.depth++
	members := make([]classMember, 0, len(n.Consts)+len(n.Methods))
	for _, c := range n.Consts {
		members = append(members, classMember{group: groupConst, span: c.Span, print: func() {
			vis := ""
			if c.Visibility != "" {
				vis = c.Visibility + " "
			}
			p.line(vis + "const " + c.Name + " = " + p.expr(c.Default) + ";")
		}})
	}
	for _, m := range n.Methods {
		members = append(members, classMember{group: groupSignature, span: p.spans[m], print: func() {
			p.line(p.signature(m))
		}})
	}
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].group != members[j].group {
			return members[i].group < members[j].group
		}
		return members[i].span.Start < members[j].span.Start
	})
	p.printMembers(members, p.stmtEnd(n))
	p.depth--
	p.line("}")
}

// signature renders a method declaration with no body, which is what an
// interface member is.
func (p *printer) signature(n *model.FuncDecl) string {
	var b strings.Builder
	if n.Visibility != "" {
		b.WriteString(n.Visibility)
		b.WriteByte(' ')
	}
	if n.Static {
		b.WriteString("static ")
	}
	b.WriteString("function ")
	b.WriteString(shortName(n.Name))
	b.WriteString("(")
	b.WriteString(p.params(n.Params))
	b.WriteString(")")
	b.WriteString(returnType(n.ReturnType))
	b.WriteString(";")
	return b.String()
}

// staticField renders a `static $name` property declaration. The modifier
// follows the visibility, which is the order PHP's own style guides use.
func (p *printer) staticField(f model.Field) string {
	visibility := f.Visibility
	if visibility == "" {
		visibility = "public"
	}
	out := visibility + " static "
	if f.Type != "" {
		out += f.Type + " "
	}
	out += "$" + f.Name
	if f.Default != nil {
		out += " = " + p.expr(f.Default)
	}
	return out
}

func (p *printer) field(f model.Field) string {
	var b strings.Builder
	if f.Visibility != "" {
		b.WriteString(f.Visibility)
		b.WriteByte(' ')
	} else {
		b.WriteString("var ")
	}
	if f.Type != "" {
		b.WriteString(f.Type)
		b.WriteByte(' ')
	}
	b.WriteString("$")
	b.WriteString(f.Name)
	if f.Default != nil {
		b.WriteString(" = ")
		b.WriteString(p.expr(f.Default))
	}
	return b.String()
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

func (p *printer) params(params []model.Param) string {
	parts := make([]string, len(params))
	for i, param := range params {
		s := ""
		if param.Modifiers != "" {
			s += param.Modifiers + " "
		}
		if param.Type != "" {
			s += param.Type + " "
		}
		if param.ByRef {
			s += "&"
		}
		if param.Variadic {
			s += "..."
		}
		s += "$" + param.Name
		if param.Default != nil {
			s += " = " + p.expr(param.Default)
		}
		parts[i] = s
	}
	return strings.Join(parts, ", ")
}

// returnType renders a `: Type` declaration, which is empty for the many
// functions that do not declare one.
func returnType(typ string) string {
	if typ == "" {
		return ""
	}
	return ": " + typ
}

func (p *printer) expr(e model.Expr) string {
	switch n := e.(type) {
	case nil:
		return ""
	case *model.Lit:
		// A parsed string literal keeps its source spelling: single quotes do
		// not interpolate and do not escape a double quote, which is what
		// makes them the readable choice for HTML and for regular expressions.
		// A literal holding a carriage return is the exception, because the
		// output has its line endings normalised, which would edit the value.
		if _, ok := n.Value.(string); ok && n.Raw != "" && !strings.Contains(n.Raw, "\r") {
			return n.Raw
		}
		return p.lit(n.Value)
	case *model.Interp:
		// An interpolated literal prints from its source for the same reason a
		// plain one does, and with more at stake: rebuilding it from the parts
		// would pick one spelling for `"$a"`, `"${a}"` and `"{$a}"`, and would
		// turn the text runs back into escapes of its own choosing.
		return n.Raw
	case *model.Var:
		if n.Const {
			return n.Name
		}
		return "$" + n.Name
	case *model.ArrayLit:
		return p.arrayLit(n)
	case *model.Index:
		if n.Index == nil {
			return p.expr(n.Base) + "[]"
		}
		return p.expr(n.Base) + "[" + p.expr(n.Index) + "]"
	case *model.PropAccess:
		return p.expr(n.Base) + "->" + n.Name
	case *model.Call:
		name := n.Name
		if n.Fallback != "" {
			name = n.Fallback
		} else {
			name = p.typeName(n.Name)
		}
		if n.Bare {
			return name
		}
		return name + "(" + p.args(n.Args) + ")"
	case *model.MethodCall:
		if n.MethodExpr != nil {
			return p.expr(n.Base) + "->" + p.expr(n.MethodExpr) + "(" + p.args(n.Args) + ")"
		}
		return p.expr(n.Base) + "->" + n.Method + "(" + p.args(n.Args) + ")"
	case *model.New:
		// The name an anonymous class carries was synthesized by the parser and
		// is not the source spelling; printing it would rewrite a working file
		// into one that does not parse. Refusing the file leaves it untouched,
		// which is what the unsupported list is for.
		if n.Decl != nil {
			return p.unsupportedNode("anonymous class", n)
		}
		class := p.typeName(n.Class)
		if n.ClassExpr != nil {
			class = p.expr(n.ClassExpr)
		}
		if len(n.Args) == 0 {
			return "new " + class
		}
		return "new " + class + "(" + p.args(n.Args) + ")"
	case *model.Unary:
		if n.Postfix {
			return p.expr(n.X) + n.Op
		}
		return n.Op + p.expr(n.X)
	case *model.Parenthesized:
		return "(" + p.expr(n.X) + ")"
	case *model.Binary:
		return p.expr(n.Left) + " " + n.Op + " " + p.expr(n.Right)
	case *model.Ternary:
		return p.expr(n.Cond) + " ? " + p.expr(n.Then) + " : " + p.expr(n.Else)
	case *model.ClassConst:
		return p.typeName(n.Class) + "::" + n.Name
	case *model.StaticProp:
		return p.typeName(n.Class) + "::$" + n.Name
	case *model.StaticCall:
		if n.MethodExpr != nil {
			return p.typeName(n.Class) + "::" + p.expr(n.MethodExpr) + "(" + p.args(n.Args) + ")"
		}
		return p.typeName(n.Class) + "::" + n.Method + "(" + p.args(n.Args) + ")"
	case *model.Invoke:
		return p.expr(n.Callee) + "(" + p.args(n.Args) + ")"
	case *model.Ref:
		return "&" + p.expr(n.X)
	case *model.Cast:
		return "(" + n.Type + ")" + p.expr(n.X)
	case *model.Closure:
		head := "function(" + p.params(n.Params) + ")"
		if n.ByRef {
			head = "function &(" + p.params(n.Params) + ")"
		}
		if n.Static {
			head = "static " + head
		}
		if len(n.Uses) > 0 {
			captures := make([]string, 0, len(n.Uses))
			for _, use := range n.Uses {
				name := "$" + use.Name
				if use.ByRef {
					name = "&" + name
				}
				captures = append(captures, name)
			}
			head += " use (" + strings.Join(captures, ", ") + ")"
		}
		head += returnType(n.ReturnType)
		return head + " " + p.inlineBlock(n.Body)
	case *model.AssignExpr:
		op := n.Op
		if op == "[]=" {
			return p.expr(n.Target) + "[] = " + p.expr(n.Value)
		}
		return p.expr(n.Target) + " " + op + " " + p.expr(n.Value)
	case *model.ListExpr:
		return "list(" + p.args(n.Elems) + ")"
	case *model.Include:
		kw := includeKeyword(n)
		if n.Parenthesized {
			return kw + "(" + p.expr(n.Path) + ")"
		}
		return kw + " " + p.expr(n.Path)
	default:
		return p.unsupportedNode("expression", e)
	}
}

func includeKeyword(n *model.Include) string {
	if n.Keyword != "" {
		return n.Keyword
	}
	if n.Once {
		return "include_once"
	}
	return "include"
}

func (p *printer) inlineBlock(body []model.Stmt) string {
	var b strings.Builder
	b.WriteString("{\n")
	p.depth++
	// Temporarily divert: build body lines with indent.
	sub := &printer{
		depth:       p.depth,
		namespace:   p.namespace,
		inPHP:       true,
		spans:       p.spans,
		comments:    p.comments,
		nextComment: p.nextComment,
	}
	sub.stmts(body)
	sub.ensurePHP()
	// The sub-printer consumed part of the shared comment stream.
	p.nextComment = sub.nextComment
	p.unsupported = append(p.unsupported, sub.unsupported...)
	p.depth--
	b.WriteString(sub.buf.String())
	b.WriteString(p.indent() + "}")
	return b.String()
}

func (p *printer) args(args []model.Expr) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if a == nil {
			parts[i] = ""
			continue
		}
		parts[i] = p.expr(a)
	}
	return strings.Join(parts, ", ")
}

const (
	// arrayLitInlineItems is the largest number of key/value pairs an array
	// literal keeps on one line. Beyond it the literal is expanded one entry
	// per line, the way gofmt expands a struct literal, because a row of three
	// or more pairs is where a single line stops being readable and stops
	// diffing well. A list of values without keys is not a record, so it is
	// only expanded when it does not fit.
	arrayLitInlineItems = 2

	// arrayLitWidth is how wide an array literal may be, counted from its own
	// indent rather than from the start of the line: the statement that holds
	// the literal is printed around it, so its width is not known here.
	arrayLitWidth = 100

	// tabWidth is the column width a hard tab is assumed to occupy when
	// measuring a line against arrayLitWidth.
	tabWidth = 4
)

func (p *printer) arrayLit(n *model.ArrayLit) string {
	if len(n.Items) == 0 {
		return "array()"
	}
	// Entries are rendered one level in, which is where they are printed when
	// the literal is expanded. A nested literal that expands therefore carries
	// the right indent already.
	p.depth++
	parts := make([]string, len(n.Items))
	keyed := false
	expand := false
	for i, it := range n.Items {
		part := p.expr(it.Val)
		if it.Key != nil {
			keyed = true
			part = p.expr(it.Key) + " => " + part
		}
		// A nested literal that expanded cannot be folded back into one line.
		expand = expand || strings.Contains(part, "\n")
		parts[i] = part
	}
	p.depth--
	inline := "array(" + strings.Join(parts, ", ") + ")"
	expand = expand ||
		(keyed && len(n.Items) > arrayLitInlineItems) ||
		p.depth*tabWidth+len(inline) > arrayLitWidth
	if !expand {
		return inline
	}
	var b strings.Builder
	b.WriteString("array(\n")
	inner := p.indent() + "\t"
	for _, part := range parts {
		b.WriteString(inner)
		b.WriteString(part)
		b.WriteString(",\n")
	}
	b.WriteString(p.indent() + ")")
	return b.String()
}

func (p *printer) lit(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%v", x)
	case string:
		return phpQuote(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

// phpQuote spells s as a PHP string literal. It is the fallback for literals
// that were built rather than parsed, so it has no source spelling to follow:
// single quotes are used when they avoid escaping, because a double-quoted
// literal would also have to escape `$` to keep PHP from interpolating it.
func phpQuote(s string) string {
	if !strings.ContainsAny(s, "'\\") && strings.ContainsAny(s, "\"$") && !strings.ContainsAny(s, "\n\r\t") {
		return "'" + s + "'"
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"', '$':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
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

func shortName(name string) string {
	if i := strings.LastIndex(name, `\`); i >= 0 {
		return name[i+1:]
	}
	return name
}

// typeName renders a class/function name relative to the file namespace when
// possible, and with a leading `\` when the stored name is absolute/global.
func (p *printer) typeName(name string) string {
	if name == "" || name == "self" || name == "static" {
		return name
	}
	if strings.HasPrefix(name, `\`) {
		return name
	}
	if p.namespace == "" {
		return name
	}
	prefix := p.namespace + `\`
	if strings.HasPrefix(name, prefix) {
		return name[len(prefix):]
	}
	if name == p.namespace {
		return shortName(name)
	}
	return `\` + name
}

func collapseBlankLines(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")
	var out []string
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if blank {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
