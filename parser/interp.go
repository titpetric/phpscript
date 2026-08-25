package parser

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// Double-quoted string interpolation.
//
// PHP spells an embedded expression two ways inside a double-quoted literal,
// and both are scanned here so that the lexer, the parser and the token_get_all
// tokenizer agree on where one ends. Simple syntax is `$name`, `$name[sub]` or
// `$name->prop`, and reaches exactly one level: `"$o->p->q"` is the property `p`
// followed by the literal text `->q`. Complex syntax is `{$expr}`, where the
// braces re-enter PHP and the expression inside is an ordinary one.
//
// A single-quoted literal never interpolates, so nothing here runs for one.

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

// interpPart is one piece of a double-quoted literal. Which fields carry a
// value depends on Kind: Text for interpText, Name with at most one of Sub and
// Prop for interpSimple, and Inner for interpCurly.
//
// Raw and Line describe the part's source. The parser reads the decoded fields
// and the tokenizer reads Raw, because token_get_all reports a literal the way
// it was written: an escape inside a text run stays an escape there, exactly as
// it does in the T_CONSTANT_ENCAPSED_STRING of a literal that embeds nothing.
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

// scanInterp walks the body of a double-quoted literal starting at src[start],
// which is the first byte after the opening quote, and splits it into parts. It
// returns the offset just past the closing quote, and whether any part is an
// embedded expression.
//
// A literal with no embedded expression still scans cleanly and reports
// interp=false, which is what lets the caller keep treating it as a plain
// string. Text parts are decoded, so the caller never decodes twice.
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
		// A literal may span lines, and both a text run and a {$...} may carry
		// the newlines. Counting over what was just consumed keeps the reported
		// line right whichever branch consumed it.
		line += strings.Count(src[prev:i], "\n")
	}
	return nil, 0, false, fmt.Errorf("line %d: unterminated string", line)
}

// scanSimple reads the simple-syntax expression starting at the `$` under
// src[i]. It reports ok=false when the `$` does not begin one, which is how a
// lone dollar such as `"$ 5"` stays literal text, as it does in PHP.
func scanSimple(src string, i int, line int) (part interpPart, end int, ok bool, err error) {
	if i+1 >= len(src) {
		return part, 0, false, nil
	}
	if src[i+1] == '{' {
		// PHP deprecated `${name}` in 8.2 and removes it in 9. Reporting it
		// beats accepting it: a literal that silently printed `${name}` back
		// would be wrong output rather than a failure the author can see.
		return part, 0, false, fmt.Errorf("line %d: ${...} string interpolation is not supported, write {$...}", line)
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

// scanCurly reads the `{$expr}` starting at the brace under src[i] and returns
// the expression source between the braces. The braces re-enter PHP, so the
// scan tracks nesting and skips over string literals: `"{$a['}']}"` closes on
// the second brace, not on the one inside the key.
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

// parseInterp builds the AST for a tInterp token. The lexer already decided the
// literal embeds an expression, and kept the source spelling in raw; the scan is
// repeated here rather than carried on the token because interpolated literals
// are a small share of the tokens in a file and the token struct is allocated
// for all of them.
func (p *parser) parseInterp(t token) (model.Expr, error) {
	// raw is the literal with its quotes, so the body starts at 1 and the scan
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

// interpSubscript resolves the key of a simple-syntax `$a[sub]`. PHP reads a
// bare word there as a string key, so `"$row[id]"` is `$row['id']` and not the
// constant `id`, which is the one place a bare word inside a subscript does not
// mean a constant.
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

// parseSubExpr parses PHP expression source that came from inside a literal. It
// runs on a parser of its own because the source is a substring rather than a
// span of the token stream, and it is handed the namespace and imports of the
// enclosing file so a name inside the braces resolves the way the same name
// resolves outside them.
func (p *parser) parseSubExpr(src string, line int) (model.Expr, error) {
	lx := newLexer(src)
	lx.inPHP = true
	toks, err := lx.run()
	if err != nil {
		return nil, fmt.Errorf("line %d: string interpolation: %w", line, err)
	}

	sub := &parser{toks: toks, namespace: p.namespace, imports: p.imports}
	e, err := sub.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("line %d: string interpolation: %w", line, err)
	}
	if sub.cur().kind != tEOF {
		return nil, fmt.Errorf("line %d: string interpolation: unexpected %s after expression", line, sub.cur())
	}
	return e, nil
}

// skipQuoted returns the offset just past the string literal opening at src[i],
// or ok=false when it is unterminated. It only has to find the end, so it reads
// `\x` as two bytes without decoding it.
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
