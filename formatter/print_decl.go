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

// The printer groups class members: constants, then properties, then methods.
// That order is what a reader of an unfamiliar class needs first, and the
// order PHP style guides settle on.
//
// groupSignature is the interface counterpart of groupMethod: a body-less
// declaration is one line and the printer keeps consecutive ones together the
// way it keeps properties, with no blank line between each.
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

// printMembers writes the members of a class or an interface body, separating
// the groups with a blank line and keeping the ones the author left between
// members of the same group. end is the line the body closes on, and is where
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
// because an interface method has no body. PHP rejects `abstract` on one and
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

// signature renders a method declaration with no body, the shape an
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
	// An imported name prints as the file imported it. The parser resolves a
	// use statement while parsing and by here the reference carries the full
	// name and the alias is only recoverable from the imports - without this,
	// formatting rewrites every `Config::get()` in the body to
	// `\Common\Config::get()` and its import goes dead.
	if alias, ok := p.imports[strings.ToLower(name)]; ok {
		return alias
	}
	return `\` + name
}
