package parser

import (
	"strings"
	"unicode"

	"github.com/titpetric/phpscript/model"
)

// This file provides a PHP-compatible tokenizer: TokenGetAll / TokenName plus
// the T_* constants. It mirrors PHP's token_get_all closely enough to drive
// minitpl's compiler (class.minitpl_compiler.php::_split_exp), which relies on
// T_VARIABLE, T_OBJECT_OPERATOR, is_array() on the elements, the token text at
// index [1], and token_name().
//
// PHP's token_get_all returns a list whose elements are either:
//   - a single-character string (e.g. "(", ")", ";"), or
//   - a 3-element array [int id, string text, int line].
//
// We reproduce that shape using the VM's own *model.Array so the result flows
// straight into transpiled PHP code (foreach, $v[0], $v[1], is_array, ...).
// Register it on a runtime with:
//
//	rt.RegisterFunc("token_get_all", parser.TokenGetAll)
//	rt.RegisterFunc("token_name", parser.TokenName)
//
// Unlike the parser's internal lexer, this tokenizer keeps whitespace, comments,
// the open/close tags, and the raw (still-quoted, still-escaped) text of every
// token, matching PHP's behaviour.

// T_* token identifiers. PHP's concrete integer values are version-specific;
// only self-consistency matters here because callers use token_name() and the
// named constants. We start at 256 like PHP (single-char tokens are < 256).
const (
	T_INLINE_HTML = 256 + iota
	T_OPEN_TAG
	T_CLOSE_TAG
	T_WHITESPACE
	T_COMMENT
	T_VARIABLE
	T_STRING // bare identifier / constant name / function name
	T_LNUMBER
	T_DNUMBER
	T_CONSTANT_ENCAPSED_STRING
	T_OBJECT_OPERATOR // ->
	T_DOUBLE_ARROW    // =>
	T_DOUBLE_COLON    // :: (a.k.a. T_PAAMAYIM_NEKUDOTAYIM)
	T_IS_EQUAL        // ==
	T_IS_NOT_EQUAL    // !=
	T_IS_IDENTICAL    // ===
	T_IS_NOT_IDENTICAL
	T_IS_SMALLER_OR_EQUAL
	T_IS_GREATER_OR_EQUAL
	T_BOOLEAN_AND // &&
	T_BOOLEAN_OR  // ||
	T_INC         // ++
	T_DEC         // --
	T_PLUS_EQUAL
	T_MINUS_EQUAL
	T_MUL_EQUAL
	T_DIV_EQUAL
	T_MOD_EQUAL
	T_CONCAT_EQUAL // .=
	// keywords
	T_IF
	T_ELSE
	T_ELSEIF
	T_FOREACH
	T_FOR
	T_WHILE
	T_AS
	T_FUNCTION
	T_RETURN
	T_NEW
	T_ECHO
	T_ARRAY
	T_CLASS
	T_VAR
	T_INCLUDE
	T_INCLUDE_ONCE
	T_REQUIRE
	T_REQUIRE_ONCE
)

// tokenNames maps each T_* id to its PHP name (what token_name returns).
var tokenNames = map[int]string{
	T_INLINE_HTML:              "T_INLINE_HTML",
	T_OPEN_TAG:                 "T_OPEN_TAG",
	T_CLOSE_TAG:                "T_CLOSE_TAG",
	T_WHITESPACE:               "T_WHITESPACE",
	T_COMMENT:                  "T_COMMENT",
	T_VARIABLE:                 "T_VARIABLE",
	T_STRING:                   "T_STRING",
	T_LNUMBER:                  "T_LNUMBER",
	T_DNUMBER:                  "T_DNUMBER",
	T_CONSTANT_ENCAPSED_STRING: "T_CONSTANT_ENCAPSED_STRING",
	T_OBJECT_OPERATOR:          "T_OBJECT_OPERATOR",
	T_DOUBLE_ARROW:             "T_DOUBLE_ARROW",
	T_DOUBLE_COLON:             "T_PAAMAYIM_NEKUDOTAYIM",
	T_IS_EQUAL:                 "T_IS_EQUAL",
	T_IS_NOT_EQUAL:             "T_IS_NOT_EQUAL",
	T_IS_IDENTICAL:             "T_IS_IDENTICAL",
	T_IS_NOT_IDENTICAL:         "T_IS_NOT_IDENTICAL",
	T_IS_SMALLER_OR_EQUAL:      "T_IS_SMALLER_OR_EQUAL",
	T_IS_GREATER_OR_EQUAL:      "T_IS_GREATER_OR_EQUAL",
	T_BOOLEAN_AND:              "T_BOOLEAN_AND",
	T_BOOLEAN_OR:               "T_BOOLEAN_OR",
	T_INC:                      "T_INC",
	T_DEC:                      "T_DEC",
	T_PLUS_EQUAL:               "T_PLUS_EQUAL",
	T_MINUS_EQUAL:              "T_MINUS_EQUAL",
	T_MUL_EQUAL:                "T_MUL_EQUAL",
	T_DIV_EQUAL:                "T_DIV_EQUAL",
	T_MOD_EQUAL:                "T_MOD_EQUAL",
	T_CONCAT_EQUAL:             "T_CONCAT_EQUAL",
	T_IF:                       "T_IF",
	T_ELSE:                     "T_ELSE",
	T_ELSEIF:                   "T_ELSEIF",
	T_FOREACH:                  "T_FOREACH",
	T_FOR:                      "T_FOR",
	T_WHILE:                    "T_WHILE",
	T_AS:                       "T_AS",
	T_FUNCTION:                 "T_FUNCTION",
	T_RETURN:                   "T_RETURN",
	T_NEW:                      "T_NEW",
	T_ECHO:                     "T_ECHO",
	T_ARRAY:                    "T_ARRAY",
	T_CLASS:                    "T_CLASS",
	T_VAR:                      "T_VAR",
	T_INCLUDE:                  "T_INCLUDE",
	T_INCLUDE_ONCE:             "T_INCLUDE_ONCE",
	T_REQUIRE:                  "T_REQUIRE",
	T_REQUIRE_ONCE:             "T_REQUIRE_ONCE",
}

// tokenKeywords maps lowercase identifiers to their keyword token id. Anything
// not listed is T_STRING (PHP treats true/false/null as T_STRING constants too).
var tokenKeywords = map[string]int{
	"if": T_IF, "else": T_ELSE, "elseif": T_ELSEIF,
	"foreach": T_FOREACH, "for": T_FOR, "while": T_WHILE, "as": T_AS,
	"function": T_FUNCTION, "return": T_RETURN, "new": T_NEW, "echo": T_ECHO,
	"array": T_ARRAY, "class": T_CLASS, "var": T_VAR,
	"include": T_INCLUDE, "include_once": T_INCLUDE_ONCE,
	"require": T_REQUIRE, "require_once": T_REQUIRE_ONCE,
}

// tokenMultiOps maps multi-character operators to their token id, longest first.
var tokenMultiOps = []struct {
	op string
	id int
}{
	{"===", T_IS_IDENTICAL}, {"!==", T_IS_NOT_IDENTICAL},
	{"==", T_IS_EQUAL}, {"!=", T_IS_NOT_EQUAL},
	{"<=", T_IS_SMALLER_OR_EQUAL}, {">=", T_IS_GREATER_OR_EQUAL},
	{"&&", T_BOOLEAN_AND}, {"||", T_BOOLEAN_OR},
	{"->", T_OBJECT_OPERATOR}, {"=>", T_DOUBLE_ARROW}, {"::", T_DOUBLE_COLON},
	{"++", T_INC}, {"--", T_DEC},
	{"+=", T_PLUS_EQUAL}, {"-=", T_MINUS_EQUAL}, {"*=", T_MUL_EQUAL},
	{"/=", T_DIV_EQUAL}, {"%=", T_MOD_EQUAL}, {".=", T_CONCAT_EQUAL},
}

// TokenName returns the PHP name for a token id (PHP's token_name). Unknown ids
// yield "UNKNOWN", matching PHP.
func TokenName(id int) string {
	if name, ok := tokenNames[id]; ok {
		return name
	}
	return "UNKNOWN"
}

// TokenGetAll tokenizes src the way PHP's token_get_all does, returning a
// *model.Array of single-char strings and [id, text, line] arrays.
func TokenGetAll(src string) *model.Array {
	tk := &phpTokenizer{src: src, line: 1}
	return tk.run()
}

type phpTokenizer struct {
	src   string
	pos   int
	line  int
	inPHP bool
	out   *model.Array
}

func (t *phpTokenizer) run() *model.Array {
	t.out = model.NewArray()
	for t.pos < len(t.src) {
		if !t.inPHP {
			t.scanInlineHTML()
			continue
		}
		t.scanPHP()
	}
	return t.out
}

// emitArr appends an [id, text, line] token.
func (t *phpTokenizer) emitArr(id int, text string, line int) {
	a := model.NewArray()
	a.Append(int64(id))
	a.Append(text)
	a.Append(int64(line))
	t.out.Append(a)
}

// emitChar appends a single-character (string) token.
func (t *phpTokenizer) emitChar(text string) { t.out.Append(text) }

func (t *phpTokenizer) scanInlineHTML() {
	start := t.pos
	startLine := t.line
	for t.pos < len(t.src) {
		if strings.HasPrefix(t.src[t.pos:], "<?php") || strings.HasPrefix(t.src[t.pos:], "<?") {
			break
		}
		t.advance()
	}
	if t.pos > start {
		t.emitArr(T_INLINE_HTML, t.src[start:t.pos], startLine)
	}
	if t.pos < len(t.src) {
		startLine = t.line
		if strings.HasPrefix(t.src[t.pos:], "<?php") {
			t.consume(5)
			t.emitArr(T_OPEN_TAG, "<?php", startLine)
		} else {
			t.consume(2)
			t.emitArr(T_OPEN_TAG, "<?", startLine)
		}
		t.inPHP = true
	}
}

func (t *phpTokenizer) scanPHP() {
	for t.pos < len(t.src) {
		if strings.HasPrefix(t.src[t.pos:], "?>") {
			startLine := t.line
			t.consume(2)
			text := "?>"
			if t.pos < len(t.src) && t.src[t.pos] == '\n' {
				t.advance()
				text = "?>\n"
			}
			t.emitArr(T_CLOSE_TAG, text, startLine)
			t.inPHP = false
			return
		}

		c := t.src[t.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			t.scanWhitespace()
		case strings.HasPrefix(t.src[t.pos:], "//") || c == '#':
			t.scanLineComment()
		case strings.HasPrefix(t.src[t.pos:], "/*"):
			t.scanBlockComment()
		case c == '$':
			t.scanVariable()
		case c == '"' || c == '\'':
			t.scanString(c)
		case isIdentStart(rune(c)):
			t.scanIdent()
		case unicode.IsDigit(rune(c)):
			t.scanNumber()
		default:
			if !t.scanOperator() {
				// Unknown byte: emit as a single-char token (PHP-ish).
				t.emitChar(string(c))
				t.advance()
			}
		}
	}
}

func (t *phpTokenizer) scanWhitespace() {
	start := t.pos
	startLine := t.line
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			break
		}
		t.advance()
	}
	t.emitArr(T_WHITESPACE, t.src[start:t.pos], startLine)
}

func (t *phpTokenizer) scanLineComment() {
	start := t.pos
	startLine := t.line
	for t.pos < len(t.src) && t.src[t.pos] != '\n' {
		if strings.HasPrefix(t.src[t.pos:], "?>") {
			break
		}
		t.advance()
	}
	t.emitArr(T_COMMENT, t.src[start:t.pos], startLine)
}

func (t *phpTokenizer) scanBlockComment() {
	start := t.pos
	startLine := t.line
	t.consume(2)
	for t.pos < len(t.src) && !strings.HasPrefix(t.src[t.pos:], "*/") {
		t.advance()
	}
	t.consume(2)
	t.emitArr(T_COMMENT, t.src[start:t.pos], startLine)
}

func (t *phpTokenizer) scanVariable() {
	start := t.pos
	startLine := t.line
	t.advance() // $
	for t.pos < len(t.src) && isIdentPart(rune(t.src[t.pos])) {
		t.advance()
	}
	// Text includes the leading $, like PHP.
	t.emitArr(T_VARIABLE, t.src[start:t.pos], startLine)
}

func (t *phpTokenizer) scanIdent() {
	start := t.pos
	startLine := t.line
	for t.pos < len(t.src) && isIdentPart(rune(t.src[t.pos])) {
		t.advance()
	}
	text := t.src[start:t.pos]
	id := T_STRING
	if kw, ok := tokenKeywords[strings.ToLower(text)]; ok {
		id = kw
	}
	t.emitArr(id, text, startLine)
}

func (t *phpTokenizer) scanNumber() {
	start := t.pos
	startLine := t.line
	isFloat := false
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		if c == '.' {
			isFloat = true
		} else if !unicode.IsDigit(rune(c)) {
			break
		}
		t.advance()
	}
	id := T_LNUMBER
	if isFloat {
		id = T_DNUMBER
	}
	t.emitArr(id, t.src[start:t.pos], startLine)
}

func (t *phpTokenizer) scanString(quote byte) {
	start := t.pos
	startLine := t.line
	t.advance() // opening quote
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		if c == '\\' && t.pos+1 < len(t.src) {
			t.consume(2)
			continue
		}
		if c == quote {
			t.advance()
			break
		}
		t.advance()
	}
	// Text keeps quotes and escapes raw, matching PHP.
	t.emitArr(T_CONSTANT_ENCAPSED_STRING, t.src[start:t.pos], startLine)
}

func (t *phpTokenizer) scanOperator() bool {
	for _, m := range tokenMultiOps {
		if strings.HasPrefix(t.src[t.pos:], m.op) {
			startLine := t.line
			t.consume(len(m.op))
			t.emitArr(m.id, m.op, startLine)
			return true
		}
	}
	const singles = "+-*/%.,;()[]{}=<>!&|?:@~^"
	c := t.src[t.pos]
	if strings.IndexByte(singles, c) >= 0 {
		t.emitChar(string(c))
		t.advance()
		return true
	}
	return false
}

func (t *phpTokenizer) advance() {
	if t.pos < len(t.src) {
		if t.src[t.pos] == '\n' {
			t.line++
		}
		t.pos++
	}
}

func (t *phpTokenizer) consume(n int) {
	for i := 0; i < n; i++ {
		t.advance()
	}
}
