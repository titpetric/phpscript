package parser

import (
	"fmt"
	"strings"
	"unicode"
)

// tokKind enumerates lexical token classes.
type tokKind int

const (
	tEOF        tokKind = iota
	tInlineHTML         // raw text outside <?php ?>
	tVar                // $name
	tIdent              // identifier / keyword
	tInt                // integer literal
	tFloat              // float literal
	tString             // quoted string literal (already unescaped)
	tOp                 // operator or punctuation (value holds the exact text)
)

// token is a single lexical unit with source position for diagnostics.
type token struct {
	kind tokKind
	val  string
	pos  int
	line int
}

func (t token) String() string { return fmt.Sprintf("%v(%q)@%d", t.kind, t.val, t.line) }

// lexer turns PHP source into tokens, switching between inline-HTML mode and
// PHP mode at <?php / ?> boundaries (the README requires file-parsing mode that
// recognises php open tags; the closing tag is optional).
type lexer struct {
	src    string
	pos    int
	line   int
	inPHP  bool
	tokens []token
}

func newLexer(src string) *lexer { return &lexer{src: src, line: 1} }

// multiCharOps are matched greedily before single-char operators.
var multiCharOps = []string{
	"===", "!==", "<<=", ">>=", "**=",
	"==", "!=", "<=", ">=", "&&", "||", "->", "=>", "++", "--",
	".=", "+=", "-=", "*=", "/=", "%=", "::",
}

func (l *lexer) run() ([]token, error) {
	l.skipShebang()
	for l.pos < len(l.src) {
		if !l.inPHP {
			l.lexInlineHTML()
			continue
		}
		if err := l.lexPHP(); err != nil {
			return nil, err
		}
	}
	l.emit(tEOF, "")
	return l.tokens, nil
}

func (l *lexer) skipShebang() {
	if !strings.HasPrefix(l.src, "#!") {
		return
	}
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.pos++
	}
	if l.pos < len(l.src) {
		l.advanceRune()
	}
}

func (l *lexer) emit(k tokKind, v string) {
	l.tokens = append(l.tokens, token{kind: k, val: v, pos: l.pos, line: l.line})
}

// lexInlineHTML consumes raw text until the next <?php (or <?) open tag.
func (l *lexer) lexInlineHTML() {
	start := l.pos
	for l.pos < len(l.src) {
		if strings.HasPrefix(l.src[l.pos:], "<?php") {
			l.flushHTML(start)
			l.advance(5)
			l.inPHP = true
			return
		}
		if strings.HasPrefix(l.src[l.pos:], "<?") {
			l.flushHTML(start)
			l.advance(2)
			l.inPHP = true
			return
		}
		l.advanceRune()
	}
	l.flushHTML(start)
}

func (l *lexer) flushHTML(start int) {
	if l.pos > start {
		l.tokens = append(l.tokens, token{kind: tInlineHTML, val: l.src[start:l.pos], pos: start, line: l.line})
	}
}

func (l *lexer) lexPHP() error {
	for l.pos < len(l.src) {
		c := l.src[l.pos]

		// Closing tag returns to inline-HTML mode.
		if strings.HasPrefix(l.src[l.pos:], "?>") {
			l.advance(2)
			// PHP eats a single trailing newline after ?>.
			if l.pos < len(l.src) && l.src[l.pos] == '\n' {
				l.advanceRune()
			}
			l.inPHP = false
			return nil
		}

		switch {
		case c == '\n':
			l.advanceRune()
		case c == ' ' || c == '\t' || c == '\r':
			l.advanceRune()
		case strings.HasPrefix(l.src[l.pos:], "//") || c == '#':
			l.skipLineComment()
		case strings.HasPrefix(l.src[l.pos:], "/*"):
			l.skipBlockComment()
		case c == '$':
			l.lexVar()
		case c == '"' || c == '\'':
			if err := l.lexString(c); err != nil {
				return err
			}
		case isIdentStart(rune(c)):
			l.lexIdent()
		case unicode.IsDigit(rune(c)):
			l.lexNumber()
		default:
			if !l.lexOperator() {
				return fmt.Errorf("line %d: unexpected character %q", l.line, c)
			}
		}
	}
	return nil
}

func (l *lexer) lexVar() {
	l.advanceRune() // consume $
	start := l.pos
	for l.pos < len(l.src) && isIdentPart(rune(l.src[l.pos])) {
		l.advanceRune()
	}
	l.emit(tVar, l.src[start:l.pos])
}

func (l *lexer) lexIdent() {
	start := l.pos
	for l.pos < len(l.src) && isIdentPart(rune(l.src[l.pos])) {
		l.advanceRune()
	}
	l.emit(tIdent, l.src[start:l.pos])
}

func (l *lexer) lexNumber() {
	start := l.pos
	isFloat := false
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '.' {
			isFloat = true
		} else if !unicode.IsDigit(rune(c)) {
			break
		}
		l.advanceRune()
	}
	if isFloat {
		l.emit(tFloat, l.src[start:l.pos])
	} else {
		l.emit(tInt, l.src[start:l.pos])
	}
}

func (l *lexer) lexString(quote byte) error {
	l.advanceRune() // opening quote
	var b strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\\' && l.pos+1 < len(l.src) {
			next := l.src[l.pos+1]
			switch next {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\', '"', '\'', '$':
				b.WriteByte(next)
			default:
				b.WriteByte('\\')
				b.WriteByte(next)
			}
			l.advance(2)
			continue
		}
		if c == quote {
			l.advanceRune()
			l.emit(tString, b.String())
			return nil
		}
		b.WriteByte(c)
		l.advanceRune()
	}
	return fmt.Errorf("line %d: unterminated string", l.line)
}

func (l *lexer) lexOperator() bool {
	for _, op := range multiCharOps {
		if strings.HasPrefix(l.src[l.pos:], op) {
			l.emit(tOp, op)
			l.advance(len(op))
			return true
		}
	}
	const singles = "+-*/%.,;()[]{}=<>!&|?:@\\"
	c := l.src[l.pos]
	if strings.IndexByte(singles, c) >= 0 {
		l.emit(tOp, string(c))
		l.advanceRune()
		return true
	}
	return false
}

func (l *lexer) skipLineComment() {
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		// stop before a closing tag so ?> still ends PHP mode
		if strings.HasPrefix(l.src[l.pos:], "?>") {
			return
		}
		l.advanceRune()
	}
}

func (l *lexer) skipBlockComment() {
	l.advance(2)
	for l.pos < len(l.src) && !strings.HasPrefix(l.src[l.pos:], "*/") {
		l.advanceRune()
	}
	l.advance(2)
}

func (l *lexer) advance(n int) {
	for i := 0; i < n && l.pos < len(l.src); i++ {
		l.advanceRune()
	}
}

func (l *lexer) advanceRune() {
	if l.pos < len(l.src) {
		if l.src[l.pos] == '\n' {
			l.line++
		}
		l.pos++
	}
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
