package parser

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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

// singleCharOps lists every one-byte operator. It is a constant, so slicing it
// (singleOpText below) yields a string header pointing into the binary's
// read-only data. Emitting an operator token allocates nothing, where
// `string(c)` allocated a fresh 1-byte string per token (rule 7).
const singleCharOps = "+-*/%.,;()[]{}=<>!&|?:@\\"

// singleOpText[c] is the token text for the single-character operator c, or ""
// when c does not start an operator. Built once at package scope (rule 7).
var singleOpText = func() [utf8.RuneSelf]string {
	var out [utf8.RuneSelf]string
	for i := 0; i < len(singleCharOps); i++ {
		out[singleCharOps[i]] = singleCharOps[i : i+1]
	}
	return out
}()

// multiOpsByFirst buckets multiCharOps by their first byte, longest first, so
// lexOperator compares against the two or three candidates that can match
// rather than walking the whole table. Longest-first ordering is what keeps
// "===" from being split into "==" and "=".
var multiOpsByFirst = func() [utf8.RuneSelf][]string {
	var out [utf8.RuneSelf][]string
	for _, op := range multiCharOps {
		c := op[0]
		out[c] = append(out[c], op)
	}
	for c := range out {
		ops := out[c]
		sort.SliceStable(ops, func(i, j int) bool { return len(ops[i]) > len(ops[j]) })
	}
	return out
}()

// bytesPerToken estimates how many source bytes one token consumes, used to
// presize the token slice (rule 6). Measured over the in-tree PHP fixtures the
// ratio is 3.7 to 5.2 bytes per token (whitespace and comments are dropped, so the
// count is well below the tokenizer's). Four is the conservative end: it costs
// a little slack on comment-heavy files and avoids the doubling, which copies
// the whole slice, on dense ones.
const bytesPerToken = 4

func (l *lexer) run() ([]token, error) {
	l.tokens = make([]token, 0, len(l.src)/bytesPerToken+8)
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
			if strings.HasPrefix(l.src[l.pos:], "\r\n") {
				l.advance(2)
			} else if l.pos < len(l.src) && (l.src[l.pos] == '\n' || l.src[l.pos] == '\r') {
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

	// Fast path: a literal with no escape sequence is a substring of the
	// source, so it needs neither a Builder nor a copy.
	start := l.pos
	for i := start; i < len(l.src); i++ {
		c := l.src[i]
		if c == '\\' {
			break
		}
		if c == quote {
			val := l.src[start:i]
			l.advance(i - start) // keeps the line counter accurate
			l.advanceRune()      // closing quote
			l.emit(tString, val)
			return nil
		}
	}

	// Slow path: the literal contains at least one escape. Presize the builder
	// (rule 6): the decoded string is never longer than the raw source up to
	// the next unescaped quote, and the first quote found is a lower bound for
	// that, so one Grow replaces the 8/16/32 doubling.
	var b strings.Builder
	if end := strings.IndexByte(l.src[start:], quote); end > 0 {
		b.Grow(end)
	}
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\\' && l.pos+1 < len(l.src) {
			l.advance(1 + l.writeEscape(&b, quote))
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

// writeEscape decodes the escape sequence starting at the backslash under
// l.pos, writes its value to b, and returns how many bytes follow the
// backslash.
//
// The two quote styles have different rules, as they do in PHP. A single-quoted
// literal recognises only `\\` and `\'`; every other backslash stands for
// itself, which is what makes `'C:\path'` and a single-quoted regex work. A
// double-quoted literal recognises the C-style escapes plus the numeric forms
// (`\x1B`, `\033`, `\u{1F600}`), and keeps the backslash for anything it does
// not recognise.
func (l *lexer) writeEscape(b *strings.Builder, quote byte) int {
	next := l.src[l.pos+1]
	if quote == '\'' {
		if next == '\\' || next == '\'' {
			b.WriteByte(next)
			return 1
		}
		b.WriteByte('\\')
		b.WriteByte(next)
		return 1
	}
	switch next {
	case 'n':
		b.WriteByte('\n')
	case 't':
		b.WriteByte('\t')
	case 'r':
		b.WriteByte('\r')
	case 'v':
		b.WriteByte('\v')
	case 'f':
		b.WriteByte('\f')
	case 'e':
		b.WriteByte(0x1b)
	case '\\', '"', '\'', '$':
		b.WriteByte(next)
	case 'x', 'X':
		// \xH or \xHH: one or two hex digits, and a lone `\x` is literal.
		digits := hexRun(l.src[l.pos+2:], 2)
		if digits == 0 {
			b.WriteByte('\\')
			b.WriteByte(next)
			return 1
		}
		value, _ := strconv.ParseUint(l.src[l.pos+2:l.pos+2+digits], 16, 16)
		b.WriteByte(byte(value))
		return 1 + digits
	case 'u':
		// \u{HHH...}: a codepoint, written out as UTF-8. Without the braces
		// PHP leaves the sequence alone.
		if l.pos+2 >= len(l.src) || l.src[l.pos+2] != '{' {
			b.WriteByte('\\')
			b.WriteByte(next)
			return 1
		}
		end := strings.IndexByte(l.src[l.pos+3:], '}')
		digits := hexRun(l.src[l.pos+3:], end)
		if end <= 0 || digits != end {
			b.WriteByte('\\')
			b.WriteByte(next)
			return 1
		}
		value, err := strconv.ParseUint(l.src[l.pos+3:l.pos+3+end], 16, 32)
		if err != nil {
			b.WriteByte('\\')
			b.WriteByte(next)
			return 1
		}
		b.WriteRune(rune(value))
		return 3 + end
	case '0', '1', '2', '3', '4', '5', '6', '7':
		// \NNN: up to three octal digits, taken mod 256 as PHP does.
		digits := octalRun(l.src[l.pos+1:], 3)
		value, _ := strconv.ParseUint(l.src[l.pos+1:l.pos+1+digits], 8, 16)
		b.WriteByte(byte(value))
		return digits
	default:
		b.WriteByte('\\')
		b.WriteByte(next)
	}
	return 1
}

// hexRun counts the leading hex digits of s, at most limit of them.
func hexRun(s string, limit int) int {
	n := 0
	for n < len(s) && n < limit {
		c := s[n]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			break
		}
		n++
	}
	return n
}

// octalRun counts the leading octal digits of s, at most limit of them.
func octalRun(s string, limit int) int {
	n := 0
	for n < len(s) && n < limit && s[n] >= '0' && s[n] <= '7' {
		n++
	}
	return n
}

// lexOperator emits one operator token, matching multi-character operators
// greedily. Every token text comes from a package-level table, so no operator
// allocates.
func (l *lexer) lexOperator() bool {
	c := l.src[l.pos]
	if c >= utf8.RuneSelf {
		return false
	}
	rest := l.src[l.pos:]
	for _, op := range multiOpsByFirst[c] {
		if strings.HasPrefix(rest, op) {
			l.emit(tOp, op)
			// Operator text never contains a newline, so the line counter
			// does not need advanceRune's per-byte check.
			l.pos += len(op)
			return true
		}
	}
	if text := singleOpText[c]; text != "" {
		l.emit(tOp, text)
		l.pos++
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
