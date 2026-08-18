// Package formatter pretty-prints phpscript ASTs in a gofmt-like style:
// hard tabs, canonical `function` keywords, parentheses on control structures,
// semicolons, collapsed blank lines, and a trailing newline. Class, function,
// and control-structure opening braces stay on the declaration line.
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
	changed, err := ChangedPaths(paths)
	return len(changed), err
}

// ChangedPaths formats each path argument in place and returns the files changed.
func ChangedPaths(paths []string) ([]string, error) {
	files, err := list.ExpandFiles(paths)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, file := range files {
		ok, err := File(file)
		if err != nil {
			return changed, err
		}
		if ok {
			changed = append(changed, file)
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
	// Files that start outside PHP are templates. Formatting their PHP blocks
	// independently is not supported yet, so leave the entire file unchanged.
	if !strings.HasPrefix(src, "<?php") {
		return src, nil
	}
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	declarationComments, attachedCommentLines := commentsBeforeDeclarations(src)
	out, unsupported := printProgram(prog, Options{
		LeadingComments:     leadingComments(src, attachedCommentLines),
		DeclarationComments: declarationComments,
	})
	// Formatting rewrites the file in place, so a node the printer cannot
	// spell has to stop the rewrite: emitting the placeholder would delete
	// working code.
	if len(unsupported) > 0 {
		return "", fmt.Errorf("cannot format: unsupported %s", strings.Join(unsupported, ", "))
	}
	out = strings.ReplaceAll(out, "\r\n", "\n")
	return strings.ReplaceAll(out, "\r", "\n"), nil
}

// Options controls AST pretty-printing.
type Options struct {
	LeadingComments     []string           // emitted after <?php, before namespace/stmts
	DeclarationComments map[int][][]string // comment groups keyed by declaration source line
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
	startsInPHP := prog.Namespace != "" || len(opts.LeadingComments) > 0 || len(prog.Stmts) == 0
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
	declarationComments := make(map[int][][]string, len(opts.DeclarationComments))
	for line, groups := range opts.DeclarationComments {
		declarationComments[line] = append([][]string(nil), groups...)
	}
	p := &printer{depth: 0, namespace: prog.Namespace, inPHP: startsInPHP, spans: prog.SourceSpans, declarationComments: declarationComments}
	if startsInPHP {
		p.line("<?php")
		p.blank()
	}
	for _, c := range opts.LeadingComments {
		p.line(c)
	}
	if len(opts.LeadingComments) > 0 {
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
		p.line("namespace " + prog.Namespace + ";")
		p.blank()
	}
	p.stmts(stmts)
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
	buf       strings.Builder
	depth     int
	namespace string
	inPHP     bool
	spans     map[model.Stmt]model.SourceSpan
	// unsupported records the AST nodes the printer has no spelling for. The
	// formatter rewrites files in place, so printing a placeholder comment for
	// one would delete working code; Source reports them as an error instead
	// and leaves the file alone.
	unsupported         []string
	declarationComments map[int][][]string
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
	for i, s := range stmts {
		if i > 0 && p.blankBetween(stmts[i-1], s) {
			p.blank()
		}
		p.stmt(s)
	}
}

func (p *printer) blankBetween(prev, next model.Stmt) bool {
	if before, ok := p.spans[next]; ok {
		if after, ok := p.spans[prev]; ok && before.Start > after.End+1 {
			return true
		}
	}
	if _, class := prev.(*model.ClassDecl); class {
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
	p.printDeclarationComments(s)
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
	case *model.Use:
		p.line(p.use(n))
	case *model.Declare:
		p.printDeclare(n)
	default:
		p.line(p.unsupportedNode("statement", s))
	}
}

func (p *printer) printDeclarationComments(s model.Stmt) {
	switch s.(type) {
	case *model.ClassDecl, *model.FuncDecl:
	default:
		return
	}
	span, ok := p.spans[s]
	if !ok {
		return
	}
	groups := p.declarationComments[span.Start]
	if len(groups) == 0 {
		return
	}
	for _, comment := range groups[0] {
		p.comment(comment)
	}
	p.declarationComments[span.Start] = groups[1:]
}

func (p *printer) comment(comment string) {
	lines := strings.Split(strings.ReplaceAll(comment, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if i > 0 {
			line = strings.TrimPrefix(line, p.indent())
		}
		p.line(line)
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
	p.depth++
	p.stmts(n.Body)
	p.depth--
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
	p.depth++
	p.stmts(n.Then)
	p.depth--
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
		p.depth++
		p.stmts(n.Else)
		p.depth--
	}
	p.line("}")
}

func (p *printer) printElseIf(n *model.If) {
	// Continuation after "} " already written without newline finish.
	p.buf.WriteString("elseif (" + p.expr(n.Cond) + ") {\n")
	p.depth++
	p.stmts(n.Then)
	p.depth--
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
		p.depth++
		p.stmts(n.Else)
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
	p.stmts(n.Body)
	p.depth--
	p.line("}")
}

func (p *printer) printFor(n *model.For) {
	// while is represented as For with nil Init/Post.
	if n.Init == nil && n.Post == nil {
		p.line("while (" + p.expr(n.Cond) + ") {")
		p.depth++
		p.stmts(n.Body)
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
	p.stmts(n.Body)
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
	p.stmts(n.Body)
	p.depth--
	p.line("}")
}

func (p *printer) printClass(n *model.ClassDecl) {
	head := "class " + shortName(n.Name)
	if n.Abstract {
		head = "abstract " + head
	}
	p.line(head + " {")
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
	for _, f := range n.Statics {
		writeBlank()
		p.line(p.staticField(f) + ";")
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
		p.printDeclarationComments(m)
		p.printFunc(m, true)
	}
	p.depth--
	p.line("}")
}

// staticField renders a `static $name` property declaration. The modifier
// follows the visibility, which is the order PHP's own style guides use.
func (p *printer) staticField(f model.Field) string {
	visibility := f.Visibility
	if visibility == "" {
		visibility = "public"
	}
	out := visibility + " static $" + f.Name
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
	p.stmts(n.Body)
	p.depth--
	for _, c := range n.Catches {
		p.line("} catch ($" + c.Var + ") {")
		p.depth++
		p.stmts(c.Body)
		p.depth--
	}
	if len(n.Finally) > 0 {
		p.line("} finally {")
		p.depth++
		p.stmts(n.Finally)
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
		p.stmts(c.Body)
		p.depth--
	}
	if len(n.Default) > 0 {
		p.line("default:")
		p.depth++
		p.stmts(n.Default)
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
		// A parsed string literal keeps its source spelling: single quotes do
		// not interpolate and do not escape a double quote, which is what
		// makes them the readable choice for HTML and for regular expressions.
		// A literal holding a carriage return is the exception, because the
		// output has its line endings normalised, which would edit the value.
		if _, ok := n.Value.(string); ok && n.Raw != "" && !strings.Contains(n.Raw, "\r") {
			return n.Raw
		}
		return p.lit(n.Value)
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
		return p.typeName(n.Class) + "::" + n.Method + "(" + p.args(n.Args) + ")"
	case *model.Invoke:
		return p.expr(n.Callee) + "(" + p.args(n.Args) + ")"
	case *model.Cast:
		return "(" + n.Type + ")" + p.expr(n.X)
	case *model.Closure:
		head := "function(" + p.params(n.Params) + ")"
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
	sub := &printer{depth: p.depth, namespace: p.namespace, inPHP: true, spans: p.spans, declarationComments: p.declarationComments}
	sub.stmts(body)
	sub.ensurePHP()
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
	case *model.ClassDecl, *model.FuncDecl:
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
	case *model.If, *model.Foreach, *model.For, *model.Try, *model.Switch:
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

// leadingComments returns // and /* */ comments between the open tag and the
// first non-comment code token (e.g. // @route annotations).
func leadingComments(src string, attached map[int]bool) []string {
	var comments []string
	seenOpen := false
	for _, val := range parser.TokenGetAll(src) {
		a, ok := val.([]any)
		if !ok {
			// CHAR token: end of preamble.
			if seenOpen {
				break
			}
			continue
		}
		switch int(a[0].(int64)) {
		case parser.T_OPEN_TAG:
			seenOpen = true
		case parser.T_WHITESPACE, parser.T_INLINE_HTML:
			// keep scanning
		case parser.T_COMMENT:
			if seenOpen && !attached[int(a[2].(int64))] {
				comments = append(comments, strings.TrimRight(a[1].(string), "\r\n"))
			}
		default:
			if seenOpen {
				return comments
			}
		}
	}
	return comments
}

// commentsBeforeDeclarations associates a run of comments with the class or
// function declaration that follows it. Whitespace, including blank lines, and
// declaration modifiers may occur between the comment and declaration.
func commentsBeforeDeclarations(src string) (map[int][][]string, map[int]bool) {
	type token struct {
		id          int
		text        string
		line        int
		commentOnly bool
	}
	raw := parser.TokenGetAll(src)
	tokens := make([]token, 0, len(raw))
	offset := 0
	for _, val := range raw {
		a, ok := val.([]any)
		if !ok {
			text := val.(string)
			tokens = append(tokens, token{text: text})
			offset += len(text)
			continue
		}
		tokenText := a[1].(string)
		lineStart := strings.LastIndex(src[:offset], "\n") + 1
		prefix := strings.TrimSpace(src[lineStart:offset])
		tokens = append(tokens, token{
			id:          int(a[0].(int64)),
			text:        tokenText,
			line:        int(a[2].(int64)),
			commentOnly: prefix == "" || prefix == "<?php" || prefix == "<?",
		})
		offset += len(tokenText)
	}

	comments := make(map[int][][]string)
	attached := make(map[int]bool)
	for i := 0; i < len(tokens); i++ {
		if tokens[i].id != parser.T_COMMENT || !tokens[i].commentOnly {
			continue
		}
		var group []token
		j := i
		for j < len(tokens) {
			if tokens[j].id == parser.T_COMMENT {
				group = append(group, tokens[j])
				j++
				continue
			}
			if tokens[j].id == parser.T_WHITESPACE {
				j++
				continue
			}
			break
		}
		declLine := 0
		for j < len(tokens) && tokens[j].id == parser.T_STRING && isDeclarationModifier(tokens[j].text) {
			if declLine == 0 {
				declLine = tokens[j].line
			}
			j++
			for j < len(tokens) && tokens[j].id == parser.T_WHITESPACE {
				j++
			}
		}
		if j >= len(tokens) || (tokens[j].id != parser.T_FUNCTION && tokens[j].id != parser.T_CLASS) {
			continue
		}
		if declLine == 0 {
			declLine = tokens[j].line
		}
		texts := make([]string, 0, len(group))
		for _, c := range group {
			texts = append(texts, strings.TrimRight(c.text, "\r\n"))
			attached[c.line] = true
		}
		comments[declLine] = append(comments[declLine], texts)
		i = j - 1
	}
	return comments, attached
}

func isDeclarationModifier(s string) bool {
	switch strings.ToLower(s) {
	case "public", "protected", "private", "static", "final", "abstract":
		return true
	}
	return false
}
