// Package formatter pretty-prints phpscript ASTs in a gofmt-like style:
// hard tabs, One True Brace Style, canonical `function` keywords, parentheses
// on control structures, semicolons, collapsed blank lines, and a trailing
// newline. Class-only files omit a closing `?>`.
package formatter

import (
	"fmt"
	"os"
	"strings"

	"github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
)

// Paths formats each path argument in place. Returns the number of files changed.
func Paths(paths []string) (int, error) {
	files, err := list.ExpandFiles(paths)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, file := range files {
		ok, err := File(file)
		if err != nil {
			return changed, err
		}
		if ok {
			changed++
		}
	}
	return changed, nil
}

// File formats path in place. Reports whether the file contents changed.
func File(path string) (bool, error) {
	in, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	out, err := Source(string(in))
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if out == string(in) {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// Source parses src and pretty-prints the AST.
func Source(src string) (string, error) {
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	return Print(prog, Options{LeadingComments: leadingComments(src)}), nil
}

// Options controls AST pretty-printing.
type Options struct {
	LeadingComments []string // emitted after <?php, before namespace/stmts
}

// Print renders prog as formatted PHP source.
func Print(prog *model.Program, opts Options) string {
	p := &printer{depth: 0, namespace: prog.Namespace}
	p.line("<?php")
	p.blank()
	for _, c := range opts.LeadingComments {
		p.line(c)
	}
	if len(opts.LeadingComments) > 0 {
		p.blank()
	}
	if prog.Namespace != "" {
		p.line("namespace " + prog.Namespace + ";")
		p.blank()
	}
	for i, s := range prog.Stmts {
		if i > 0 && (isDecl(s) || isDecl(prog.Stmts[i-1])) {
			p.blank()
		}
		p.stmt(s)
	}
	out := collapseBlankLines(p.buf.String())
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

type printer struct {
	buf       strings.Builder
	depth     int
	namespace string
}

func (p *printer) indent() string {
	return strings.Repeat("\t", p.depth)
}

func (p *printer) line(s string) {
	p.buf.WriteString(p.indent())
	p.buf.WriteString(s)
	p.buf.WriteByte('\n')
}

func (p *printer) blank() {
	p.buf.WriteByte('\n')
}

func (p *printer) stmt(s model.Stmt) {
	switch n := s.(type) {
	case *model.InlineHTML:
		// Raw HTML outside PHP tags — emit verbatim without indent.
		p.buf.WriteString(n.Text)
		if !strings.HasSuffix(n.Text, "\n") {
			p.buf.WriteByte('\n')
		}
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
	case *model.Return:
		if n.Value == nil {
			p.line("return;")
		} else {
			p.line("return " + p.expr(n.Value) + ";")
		}
	case *model.Include:
		kw := "include"
		if n.Once {
			kw = "include_once"
		}
		p.line(kw + " " + p.expr(n.Path) + ";")
	case *model.FuncDecl:
		p.printFunc(n, false)
	case *model.ClassDecl:
		p.printClass(n)
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
	default:
		p.line(fmt.Sprintf("/* unsupported statement %T */", s))
	}
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
	p.depth++
	for _, s := range n.Then {
		p.stmt(s)
	}
	p.depth--
	if len(n.Else) == 1 {
		if nested, ok := n.Else[0].(*model.If); ok {
			// Write "} elseif ..." without a blank close line.
			p.buf.WriteString(p.indent() + "} ")
			// printIf for elseif writes the full line including indent — trim.
			p.printElseIf(nested)
			return
		}
	}
	if len(n.Else) > 0 {
		p.line("} else {")
		p.depth++
		for _, s := range n.Else {
			p.stmt(s)
		}
		p.depth--
	}
	p.line("}")
}

func (p *printer) printElseIf(n *model.If) {
	// Continuation after "} " already written without newline finish.
	p.buf.WriteString("elseif (" + p.expr(n.Cond) + ") {\n")
	p.depth++
	for _, s := range n.Then {
		p.stmt(s)
	}
	p.depth--
	if len(n.Else) == 1 {
		if nested, ok := n.Else[0].(*model.If); ok {
			p.buf.WriteString(p.indent() + "} ")
			p.printElseIf(nested)
			return
		}
	}
	if len(n.Else) > 0 {
		p.line("} else {")
		p.depth++
		for _, s := range n.Else {
			p.stmt(s)
		}
		p.depth--
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
	p.depth++
	for _, s := range n.Body {
		p.stmt(s)
	}
	p.depth--
	p.line("}")
}

func (p *printer) printFor(n *model.For) {
	// while is represented as For with nil Init/Post.
	if n.Init == nil && n.Post == nil {
		p.line("while (" + p.expr(n.Cond) + ") {")
		p.depth++
		for _, s := range n.Body {
			p.stmt(s)
		}
		p.depth--
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
	p.depth++
	for _, s := range n.Body {
		p.stmt(s)
	}
	p.depth--
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
	if n.Class != "" && !inClass {
		b.WriteString(shortName(n.Class))
		b.WriteString("::")
	}
	b.WriteString(shortName(n.Name))
	b.WriteString("(")
	b.WriteString(p.params(n.Params))
	b.WriteString(")")
	if n.Abstract {
		b.WriteString(";")
		p.line(b.String())
		return
	}
	b.WriteString(" {")
	p.line(b.String())
	p.depth++
	for _, s := range n.Body {
		p.stmt(s)
	}
	p.depth--
	p.line("}")
}

func (p *printer) printClass(n *model.ClassDecl) {
	head := "class " + shortName(n.Name) + " {"
	if n.Abstract {
		head = "abstract " + head
	}
	p.line(head)
	p.depth++
	first := true
	writeBlank := func() {
		if !first {
			p.blank()
		}
		first = false
	}
	for _, f := range n.Fields {
		writeBlank()
		p.line(p.field(f) + ";")
	}
	for _, c := range n.Consts {
		writeBlank()
		vis := ""
		if c.Visibility != "" {
			vis = c.Visibility + " "
		}
		p.line(vis + "const " + c.Name + " = " + p.expr(c.Default) + ";")
	}
	for _, m := range n.Methods {
		writeBlank()
		p.printFunc(m, true)
	}
	p.depth--
	p.line("}")
}

func (p *printer) field(f model.Field) string {
	var b strings.Builder
	if f.Visibility != "" {
		b.WriteString(f.Visibility)
		b.WriteByte(' ')
	} else {
		b.WriteString("var ")
	}
	b.WriteString("$")
	b.WriteString(f.Name)
	if f.Default != nil {
		b.WriteString(" = ")
		b.WriteString(p.expr(f.Default))
	}
	return b.String()
}

func (p *printer) printTry(n *model.Try) {
	p.line("try {")
	p.depth++
	for _, s := range n.Body {
		p.stmt(s)
	}
	p.depth--
	for _, c := range n.Catches {
		p.line("} catch ($" + c.Var + ") {")
		p.depth++
		for _, s := range c.Body {
			p.stmt(s)
		}
		p.depth--
	}
	if len(n.Finally) > 0 {
		p.line("} finally {")
		p.depth++
		for _, s := range n.Finally {
			p.stmt(s)
		}
		p.depth--
	}
	p.line("}")
}

func (p *printer) printSwitch(n *model.Switch) {
	p.line("switch (" + p.expr(n.Cond) + ") {")
	p.depth++
	for _, c := range n.Cases {
		p.line("case " + p.expr(c.Value) + ":")
		p.depth++
		for _, s := range c.Body {
			p.stmt(s)
		}
		p.depth--
	}
	if len(n.Default) > 0 {
		p.line("default:")
		p.depth++
		for _, s := range n.Default {
			p.stmt(s)
		}
		p.depth--
	}
	p.depth--
	p.line("}")
}

func (p *printer) params(params []model.Param) string {
	parts := make([]string, len(params))
	for i, param := range params {
		s := "$" + param.Name
		if param.Default != nil {
			s += " = " + p.expr(param.Default)
		}
		parts[i] = s
	}
	return strings.Join(parts, ", ")
}

func (p *printer) expr(e model.Expr) string {
	switch n := e.(type) {
	case nil:
		return ""
	case *model.Lit:
		return p.lit(n.Value)
	case *model.Var:
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
		return name + "(" + p.args(n.Args) + ")"
	case *model.MethodCall:
		return p.expr(n.Base) + "->" + n.Method + "(" + p.args(n.Args) + ")"
	case *model.New:
		class := p.typeName(n.Class)
		if len(n.Args) == 0 {
			return "new " + class
		}
		return "new " + class + "(" + p.args(n.Args) + ")"
	case *model.Unary:
		if n.Postfix {
			return p.expr(n.X) + n.Op
		}
		return n.Op + p.expr(n.X)
	case *model.Binary:
		return p.expr(n.Left) + " " + n.Op + " " + p.expr(n.Right)
	case *model.Ternary:
		return p.expr(n.Cond) + " ? " + p.expr(n.Then) + " : " + p.expr(n.Else)
	case *model.ClassConst:
		return p.typeName(n.Class) + "::" + n.Name
	case *model.Cast:
		return "(" + n.Type + ")" + p.expr(n.X)
	case *model.Closure:
		return "function(" + p.params(n.Params) + ") " + p.inlineBlock(n.Body)
	case *model.AssignExpr:
		op := n.Op
		if op == "[]=" {
			return p.expr(n.Target) + "[] = " + p.expr(n.Value)
		}
		return "(" + p.expr(n.Target) + " " + op + " " + p.expr(n.Value) + ")"
	case *model.ListExpr:
		return "list(" + p.args(n.Elems) + ")"
	case *model.Include:
		kw := "include"
		if n.Once {
			kw = "include_once"
		}
		return kw + "(" + p.expr(n.Path) + ")"
	default:
		return fmt.Sprintf("/* expr %T */", e)
	}
}

func (p *printer) inlineBlock(body []model.Stmt) string {
	var b strings.Builder
	b.WriteString("{\n")
	p.depth++
	// Temporarily divert: build body lines with indent.
	sub := &printer{depth: p.depth, namespace: p.namespace}
	for _, s := range body {
		sub.stmt(s)
	}
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

func (p *printer) arrayLit(n *model.ArrayLit) string {
	parts := make([]string, len(n.Items))
	for i, it := range n.Items {
		if it.Key != nil {
			parts[i] = p.expr(it.Key) + " => " + p.expr(it.Val)
		} else {
			parts[i] = p.expr(it.Val)
		}
	}
	return "array(" + strings.Join(parts, ", ") + ")"
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

func phpQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
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
	case *model.ClassDecl, *model.FuncDecl:
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
	lines := strings.Split(src, "\n")
	var out []string
	blank := false
	for _, line := range lines {
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

// leadingComments returns // and /* */ comments between the open tag and the
// first non-comment code token (e.g. // @route annotations).
func leadingComments(src string) []string {
	var comments []string
	seenOpen := false
	parser.TokenGetAll(src).Range(func(_, val any) bool {
		a, ok := val.(*model.Array)
		if !ok {
			// CHAR token — end of preamble.
			if seenOpen {
				return false
			}
			return true
		}
		id, _ := a.Get(int64(0))
		text, _ := a.Get(int64(1))
		switch int(id.(int64)) {
		case parser.T_OPEN_TAG:
			seenOpen = true
		case parser.T_WHITESPACE, parser.T_INLINE_HTML:
			// keep scanning
		case parser.T_COMMENT:
			if seenOpen {
				comments = append(comments, strings.TrimRight(text.(string), "\r\n"))
			}
		default:
			if seenOpen {
				return false
			}
		}
		return true
	})
	return comments
}
