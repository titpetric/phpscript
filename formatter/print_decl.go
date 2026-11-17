package formatter

import (
	"sort"
	"strings"

	"github.com/titpetric/phpscript/model"
)

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

// The printer groups class members as constants, properties, then methods.
const (
	groupConst = iota
	groupProp
	groupSignature
	groupMethod
)

// classMember holds one declaration, its group and the source lines it occupied.
type classMember struct {
	group int
	span  model.SourceSpan
	print func()
}

func (p *printer) printClass(n *model.ClassDecl) {
	// This prepends modifiers in reverse of their printed order, which puts
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
	// Properties keep their declaration order; the parser collects
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

// printMembers writes the members, separating groups, and flushes comments up to end.
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

// printInterface renders constants then signatures, never printing the abstract PHP rejects.
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

// signature renders a method declaration with no body, as an interface member.
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

// staticField renders a static property, the modifier after the visibility.
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

// returnType renders a return type as ': Type', empty when undeclared.
func returnType(typ string) string {
	if typ == "" {
		return ""
	}
	return ": " + typ
}

func shortName(name string) string {
	if i := strings.LastIndex(name, `\`); i >= 0 {
		return name[i+1:]
	}
	return name
}

// typeName renders a name relative to the file namespace, absolute names keeping their leading backslash.
func (p *printer) typeName(name string) string {
	if name == "" || name == "self" || name == "static" {
		return name
	}
	// A member of an imported go package prints as the source wrote it,
	// whatever namespace the file declares. See goimport.go.
	if member, ok := p.goName(name); ok {
		return member
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
	// A stored name equal to the namespace is an absolute reference: a
	// relative bare name inside `namespace Task` qualifies to `Task\Task`,
	// `Task` here can only come from `\Task`; shortening it
	// would change which class the call names.
	// An imported name prints as the file imported it. The parser resolves a
	// use statement while parsing and by here the reference holds the full
	// name and the alias is only recoverable from the imports - without this,
	// formatting rewrites every `Config::get()` in the body to
	// `\Common\Config::get()` and its import goes dead.
	if alias, ok := p.imports[strings.ToLower(name)]; ok {
		return alias
	}
	return `\` + name
}
