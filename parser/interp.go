package parser

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// Double-quoted string interpolation.
//
// PHP spells an embedded expression two ways inside a double-quoted literal.
// This file scans both and the lexer, the parser and the token_get_all
// tokenizer agree on where one ends. Simple syntax is `$name`, `$name[sub]` or
// `$name->prop`, and reaches exactly one level: `"$o->p->q"` is the property `p`
// followed by the literal text `->q`. Complex syntax is `{$expr}`, where the
// braces re-enter PHP and the expression inside is an ordinary one.
//
// A single-quoted literal never interpolates and nothing here runs for one.

// interpKind classifies one piece of a double-quoted literal.
type interpKind uint8

const (
	// interpText is a run of literal text, escapes already decoded.
	interpText interpKind = iota
	// interpSimple is `$name`, `$name[sub]` or `$name->prop`.
	interpSimple
	// interpCurly is `{$expr}`.
	interpCurly
)

// interpPart holds one piece of a double-quoted literal, its fields set by Kind.
type interpPart struct {
	Kind  interpKind
	Text  string
	Name  string
	Sub   string
	Prop  string
	Inner string
	Raw   string
	Line  int
}

// scanInterp splits a double-quoted literal into parts and reports its end offset and interp.
func scanInterp(src string, start int, line int) (parts []interpPart, end int, interp bool, err error) {
	var text strings.Builder
	textStart, textLine := start, line
	flush := func(at int) {
		if at > textStart {
			parts = append(parts, interpPart{
				Kind: interpText,
				Text: text.String(),
				Raw:  src[textStart:at],
				Line: textLine,
			})
			text.Reset()
		}
	}

	i := start
	for i < len(src) {
		c := src[i]
		prev := i
		switch {
		case c == '\\' && i+1 < len(src):
			i += 1 + decodeEscape(src, i, '"', &text)

		case c == '"':
			flush(i)
			return parts, i + 1, interp, nil

		case c == '$':
			part, next, ok, perr := scanSimple(src, i, line)
			if perr != nil {
				return nil, 0, false, perr
			}
			if !ok {
				text.WriteByte(c)
				i++
				continue
			}
			flush(i)
			part.Raw, part.Line = src[i:next], line
			parts = append(parts, part)
			interp = true
			i, textStart, textLine = next, next, line

		case c == '{' && i+1 < len(src) && src[i+1] == '$':
			inner, next, cerr := scanCurly(src, i, line)
			if cerr != nil {
				return nil, 0, false, cerr
			}
			flush(i)
			parts = append(parts, interpPart{
				Kind:  interpCurly,
				Inner: inner,
				Raw:   src[i:next],
				Line:  line,
			})
			interp = true
			i, textStart, textLine = next, next, line

		default:
			text.WriteByte(c)
			i++
		}
		// A literal may span lines, and both a text run and a {$...} may hold
		// the newlines. Counting over the consumed span keeps the reported
		// line right whichever branch consumed it.
		line += strings.Count(src[prev:i], "\n")
	}
	return nil, 0, false, fmt.Errorf("line %d: unterminated string", line)
}

// scanSimple reads the simple-syntax expression at src[i], ok=false for a lone dollar.
func scanSimple(src string, i int, line int) (part interpPart, end int, ok bool, err error) {
	if i+1 >= len(src) {
		return part, 0, false, nil
	}
	if src[i+1] == '{' {
		return scanDollarBrace(src, i, line)
	}
	if !isIdentStart(rune(src[i+1])) {
		return part, 0, false, nil
	}

	j := i + 1
	for j < len(src) && isIdentPart(rune(src[j])) {
		j++
	}
	part = interpPart{Kind: interpSimple, Name: src[i+1 : j]}

	switch {
	case j < len(src) && src[j] == '[':
		// The simple subscript is one token: a bare word, an optionally signed
		// number, or a variable. Anything else, `$a[foo()]` among them, needs
		// the braces.
		k := strings.IndexByte(src[j:], ']')
		if k < 0 {
			return part, 0, false, fmt.Errorf("line %d: unterminated [ in string interpolation", line)
		}
		part.Sub = src[j+1 : j+k]
		return part, j + k + 1, true, nil

	case j+2 < len(src) && src[j] == '-' && src[j+1] == '>' && isIdentStart(rune(src[j+2])):
		k := j + 2
		for k < len(src) && isIdentPart(rune(src[k])) {
			k++
		}
		part.Prop = src[j+2 : k]
		return part, k, true, nil
	}
	return part, j, true, nil
}

// scanCurly reads the expression in {$expr}, tracking nesting and skipping string literals.
func scanCurly(src string, i int, line int) (inner string, end int, err error) {
	depth := 0
	for j := i; j < len(src); j++ {
		switch src[j] {
		case '\'', '"':
			k, ok := skipQuoted(src, j)
			if !ok {
				return "", 0, fmt.Errorf("line %d: unterminated string in {$...} interpolation", line)
			}
			j = k - 1
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[i+1 : j], j + 1, nil
			}
		}
	}
	return "", 0, fmt.Errorf("line %d: unterminated {$...} string interpolation", line)
}

// parseInterp builds the AST for a tInterp token, scanning its raw source again.
func (p *parser) parseInterp(t token) (model.Expr, error) {
	// raw is the literal with its quotes and the body starts at 1 and the scan
	// stops on the closing quote the lexer already found.
	parts, _, _, err := scanInterp(t.raw, 1, t.line)
	if err != nil {
		return nil, err
	}

	out := make([]model.Expr, 0, len(parts))
	for _, part := range parts {
		e, err := p.interpExpr(part, t.line)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return &model.Interp{Parts: out, Raw: t.raw}, nil
}

// interpExpr turns one scanned part into an expression.
func (p *parser) interpExpr(part interpPart, line int) (model.Expr, error) {
	switch part.Kind {
	case interpText:
		return p.newLit(part.Text), nil

	case interpSimple:
		base := model.Expr(p.newVar(part.Name))
		switch {
		case part.Prop != "":
			return p.newProp(base, part.Prop), nil
		case part.Sub != "":
			key, err := p.interpSubscript(part.Sub, line)
			if err != nil {
				return nil, err
			}
			return p.newIndex(base, key), nil
		}
		return base, nil

	default:
		return p.parseSubExpr(part.Inner, line)
	}
}

// interpSubscript resolves a simple-syntax $a[sub] key, a bare word reading as a string.
func (p *parser) interpSubscript(sub string, line int) (model.Expr, error) {
	if sub == "" {
		return nil, fmt.Errorf("line %d: empty [] in string interpolation", line)
	}
	if isIdentStart(rune(sub[0])) {
		for i := 0; i < len(sub); i++ {
			if !isIdentPart(rune(sub[i])) {
				return nil, fmt.Errorf("line %d: %q is not a simple subscript, write {$...}", line, sub)
			}
		}
		return p.newStringLit(sub, ""), nil
	}
	return p.parseSubExpr(sub, line)
}

// parseSubExpr parses expression source from inside a literal, on a parser holding the file's namespace.
func (p *parser) parseSubExpr(src string, line int) (model.Expr, error) {
	lx := newLexer(src)
	lx.inPHP = true
	toks, err := lx.run()
	if err != nil {
		return nil, fmt.Errorf("line %d: string interpolation: %w", line, err)
	}
	defer releaseTokens(toks)

	sub := &parser{toks: toks, namespace: p.namespace, imports: p.imports, goRuntime: p.goRuntime, goMode: p.goMode}
	e, err := sub.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("line %d: string interpolation: %w", line, err)
	}
	if sub.cur().kind != tEOF {
		return nil, fmt.Errorf("line %d: string interpolation: unexpected %s after expression", line, sub.cur())
	}
	return e, nil
}

// skipQuoted returns the offset past the literal opening at src[i], ok=false when unterminated.
func skipQuoted(src string, i int) (end int, ok bool) {
	quote := src[i]
	for j := i + 1; j < len(src); j++ {
		if src[j] == '\\' && j+1 < len(src) {
			j++
			continue
		}
		if src[j] == quote {
			return j + 1, true
		}
	}
	return 0, false
}

// scanDollarBrace reads the ${name} and ${name[subscript]} forms starting at the `$` under src[i].
func scanDollarBrace(src string, i int, line int) (part interpPart, end int, ok bool, err error) {
	j := i + 2
	if j >= len(src) || !isIdentStart(rune(src[j])) {
		return part, 0, false, dollarBraceErr(line)
	}
	start := j
	for j < len(src) && isIdentPart(rune(src[j])) {
		j++
	}
	part = interpPart{Kind: interpSimple, Name: src[start:j]}

	if j < len(src) && src[j] == '[' {
		k := strings.IndexByte(src[j:], ']')
		if k < 0 {
			return part, 0, false, fmt.Errorf("line %d: unterminated [ in string interpolation", line)
		}
		part.Sub = src[j+1 : j+k]
		j += k + 1
	}

	if j >= len(src) || src[j] != '}' {
		return part, 0, false, dollarBraceErr(line)
	}
	return part, j + 1, true, nil
}

// dollarBraceErr reports a ${...} that names the variable-variable form, which is not implemented.
func dollarBraceErr(line int) error {
	return fmt.Errorf("line %d: ${expression} names a variable by its value and needs $$name, which is not implemented; write {$...} for an expression", line)
}
