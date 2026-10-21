package formatter

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

func (p *printer) expr(e model.Expr) string {
	switch n := e.(type) {
	case nil:
		return ""
	case *model.Lit:
		// A parsed string literal keeps its source spelling: single quotes do
		// not interpolate and do not escape a double quote, as
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
		return name + "(" + p.callArgs(n.FirstClass, n.Args) + ")"
	case *model.MethodCall:
		if n.MethodExpr != nil {
			return p.expr(n.Base) + "->" + p.expr(n.MethodExpr) + "(" + p.callArgs(n.FirstClass, n.Args) + ")"
		}
		return p.expr(n.Base) + "->" + n.Method + "(" + p.callArgs(n.FirstClass, n.Args) + ")"
	case *model.New:
		// The name an anonymous class carries comes from the parser and
		// is not the source spelling; printing it would rewrite a working file
		// into one that does not parse. Refusing the file leaves it untouched,
		// and the unsupported list is for that.
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
			return p.typeName(n.Class) + "::" + p.expr(n.MethodExpr) + "(" + p.callArgs(n.FirstClass, n.Args) + ")"
		}
		return p.typeName(n.Class) + "::" + n.Method + "(" + p.callArgs(n.FirstClass, n.Args) + ")"
	case *model.Invoke:
		return p.expr(n.Callee) + "(" + p.callArgs(n.FirstClass, n.Args) + ")"
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

// callArgs prints the argument list of a call, which for PHP 8.1's first-class
// callable syntax is the literal ellipsis. The node carries the spelling and
// no desugared form, so the printer writes `greet(...)` back as itself.
func (p *printer) callArgs(firstClass bool, args []model.Expr) string {
	if firstClass {
		return "..."
	}
	return p.args(args)
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
	// literal keeps on one line. Past that the printer expands the literal one
	// entry per line, the way gofmt expands a struct literal, because a row of
	// three or more pairs is where a single line stops being readable and
	// stops diffing well. A list of values without keys is not a record, so
	// the printer expands one only where it does not fit.
	arrayLitInlineItems = 2

	// arrayLitWidth is how wide an array literal may be, counted from its own
	// indent and not from the start of the line: the printer writes the
	// statement around the literal, so this code cannot see that width.
	arrayLitWidth = 100

	// tabWidth is the column width a hard tab counts as when measuring a line
	// against arrayLitWidth.
	tabWidth = 4
)

func (p *printer) arrayLit(n *model.ArrayLit) string {
	if len(n.Items) == 0 {
		return "array()"
	}
	// This renders entries one level in, the position an expanded literal
	// prints them at. A nested literal that expands then carries the right
	// indent already.
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
		// Nothing folds a nested literal that expanded back onto one line.
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

// phpQuote spells s as a PHP string literal, the fallback for a literal a
// caller built without parsing, where there is no source spelling to follow:
// it picks single quotes where they avoid escaping, because a double-quoted
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
