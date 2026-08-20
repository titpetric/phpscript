package parser

import (
	"strings"
	"testing"
)

// lexAll runs the lexer over src and fails the test on error.
func lexAll(t *testing.T, src string) []token {
	t.Helper()
	toks, err := newLexer(src).run()
	if err != nil {
		t.Fatalf("lex %q: %v", src, err)
	}
	return toks
}

// singleCharOperators lists every one-byte operator lexOperator accepts. It is
// spelled out here (rather than referencing the implementation constant) so the
// test fails if the set silently changes.
const singleCharOperators = "+-*/%.,;()[]{}=<>!&|?:@\\"

func TestLexSingleCharOperators(t *testing.T) {
	for i := 0; i < len(singleCharOperators); i++ {
		op := singleCharOperators[i : i+1]
		toks := lexAll(t, "<?php "+op)
		if len(toks) != 2 {
			t.Fatalf("%q: got %d tokens, want 2: %v", op, len(toks), toks)
		}
		if toks[0].kind != tOp || toks[0].val != op {
			t.Errorf("%q: got %v, want tOp(%q)", op, toks[0], op)
		}
		if toks[1].kind != tEOF {
			t.Errorf("%q: last token %v, want tEOF", op, toks[1])
		}
	}
}

func TestLexMultiCharOperators(t *testing.T) {
	for _, op := range multiCharOps {
		toks := lexAll(t, "<?php "+op)
		if len(toks) != 2 {
			t.Fatalf("%q: got %d tokens, want 2: %v", op, len(toks), toks)
		}
		if toks[0].kind != tOp || toks[0].val != op {
			t.Errorf("%q: got %v, want tOp(%q) (mis-split?)", op, toks[0], op)
		}
	}
}

// TestLexOperatorGreedy pins the boundaries between operators that share a
// prefix, so a table reordering cannot silently split "===" into "==" + "=".
func TestLexOperatorGreedy(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"===", []string{"==="}},
		{"====", []string{"===", "="}},
		{"==", []string{"=="}},
		{"=", []string{"="}},
		{"!==", []string{"!=="}},
		{"!=", []string{"!="}},
		{"!", []string{"!"}},
		{"<=", []string{"<="}},
		{"<<=", []string{"<<="}},
		{"<<", []string{"<", "<"}},
		{">>=", []string{">>="}},
		{">=", []string{">="}},
		{"**=", []string{"**="}},
		{"**", []string{"**"}},
		{"->", []string{"->"}},
		{"-", []string{"-"}},
		{"--", []string{"--"}},
		{"---", []string{"--", "-"}},
		{"=>", []string{"=>"}},
		{"++", []string{"++"}},
		{"+=", []string{"+="}},
		{".=", []string{".="}},
		{"..", []string{".", "."}},
		{"::", []string{"::"}},
		{":", []string{":"}},
		{":::", []string{"::", ":"}},
		{"&&", []string{"&&"}},
		{"&", []string{"&"}},
		{"||", []string{"||"}},
		{"|", []string{"|"}},
		{"*=", []string{"*="}},
		{"/=", []string{"/="}},
		{"%=", []string{"%="}},
		{"-=", []string{"-="}},
	}
	for _, tc := range cases {
		toks := lexAll(t, "<?php "+tc.src)
		var got []string
		for _, tok := range toks {
			if tok.kind == tEOF {
				break
			}
			if tok.kind != tOp {
				t.Fatalf("%q: unexpected non-operator token %v", tc.src, tok)
			}
			got = append(got, tok.val)
		}
		if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
			t.Errorf("%q: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestLexOperatorNoAlloc is the regression test for the boxed operator table:
// emitting an operator token must not allocate a string.
func TestLexOperatorNoAlloc(t *testing.T) {
	for _, src := range []string{
		strings.Repeat("(", 64),   // single-char
		strings.Repeat(";", 64),   // single-char
		strings.Repeat("+", 64),   // lexes as "++" pairs
		strings.Repeat("===", 64), // multi-char, longest match
		strings.Repeat("->", 64),  // multi-char
	} {
		l := &lexer{src: src, line: 1, inPHP: true, tokens: make([]token, 0, 256)}
		allocs := testing.AllocsPerRun(50, func() {
			l.pos = 0
			l.tokens = l.tokens[:0]
			for l.pos < len(l.src) {
				if !l.lexOperator() {
					t.Fatalf("lexOperator rejected %q", l.src[l.pos])
				}
			}
		})
		if allocs != 0 {
			t.Errorf("lexing %q...: %.0f allocs/run, want 0", src[:3], allocs)
		}
	}
}

func TestLexOperatorPositionsAndLines(t *testing.T) {
	src := "<?php\n$a->b;\n$c[1] = 2;\n"
	toks := lexAll(t, src)
	type want struct {
		kind tokKind
		val  string
		line int
	}
	wants := []want{
		{tVar, "a", 2},
		{tOp, "->", 2},
		{tIdent, "b", 2},
		{tOp, ";", 2},
		{tVar, "c", 3},
		{tOp, "[", 3},
		{tInt, "1", 3},
		{tOp, "]", 3},
		{tOp, "=", 3},
		{tInt, "2", 3},
		{tOp, ";", 3},
		{tEOF, "", 4},
	}
	if len(toks) != len(wants) {
		t.Fatalf("got %d tokens, want %d: %v", len(toks), len(wants), toks)
	}
	for i, w := range wants {
		got := toks[i]
		if got.kind != w.kind || got.val != w.val || got.line != w.line {
			t.Errorf("token %d: got %v line %d, want %v(%q) line %d", i, got, got.line, w.kind, w.val, w.line)
		}
	}
}

func TestLexLineNumbersAcrossConstructs(t *testing.T) {
	src := "a\n<?php\n// comment\n/* two\nlines */\n$x = \"str\";\n?>\nhtml\n<?php $y = 1;"
	toks := lexAll(t, src)
	byVal := map[string]int{}
	for _, tok := range toks {
		if tok.kind == tVar {
			byVal[tok.val] = tok.line
		}
	}
	if byVal["x"] != 6 {
		t.Errorf("$x on line %d, want 6", byVal["x"])
	}
	if byVal["y"] != 9 {
		t.Errorf("$y on line %d, want 9", byVal["y"])
	}
}

func TestLexRejectsUnknownCharacter(t *testing.T) {
	if _, err := newLexer("<?php ~").run(); err == nil {
		t.Fatal("expected an error for an unsupported character")
	}
}

// TestLexNumberLiterals covers PHP's integer and float spellings. The lexer
// decides where a literal ends and whether it is a float; what base a prefix
// means is numLit's job, which TestNumLitBases covers.
func TestLexNumberLiterals(t *testing.T) {
	cases := []struct {
		src  string
		kind tokKind
		val  string
	}{
		{"1", tInt, "1"},
		{"017", tInt, "017"},
		{"0x1F", tInt, "0x1F"},
		{"0XdeadBEEF", tInt, "0XdeadBEEF"},
		{"0b1010", tInt, "0b1010"},
		{"0o17", tInt, "0o17"},
		{"1_000_000", tInt, "1_000_000"},
		{"1.5", tFloat, "1.5"},
		{"1.", tFloat, "1."},
		{"1e3", tFloat, "1e3"},
		{"1E-3", tFloat, "1E-3"},
		{"1.5e+10", tFloat, "1.5e+10"},
		// A hex literal owns its digits: the "e" in 0x1e is one of them, not
		// the start of an exponent.
		{"0x1e", tInt, "0x1e"},
	}
	for _, c := range cases {
		toks := lexAll(t, "<?php "+c.src)
		if len(toks) != 2 {
			t.Errorf("%q: got %d tokens, want 2: %v", c.src, len(toks), toks)
			continue
		}
		if toks[0].kind != c.kind || toks[0].val != c.val {
			t.Errorf("%q: got %v(%q), want %v(%q)", c.src, toks[0].kind, toks[0].val, c.kind, c.val)
		}
	}
}

// TestLexNumberBoundaries checks where a number stops, for the spellings that
// end next to something the lexer must not swallow.
func TestLexNumberBoundaries(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		// A prefix with no digits after it is the integer 0 and an identifier.
		{"0xg", []string{"0", "xg"}},
		{"1 . 2", []string{"1", ".", "2"}},
		{"0b12", []string{"0b1", "2"}},
	}
	for _, c := range cases {
		toks := lexAll(t, "<?php "+c.src)
		var got []string
		for _, tok := range toks[:len(toks)-1] {
			got = append(got, tok.val)
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%q: got %v, want %v", c.src, got, c.want)
		}
	}
}

// TestNumLitBases checks the values the parser hands the runtime, including the
// literal too large for an int, which PHP widens to a float rather than
// rejecting.
func TestNumLitBases(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{"1", int64(1)},
		{"017", int64(15)},
		{"0o17", int64(15)},
		{"0x1F", int64(31)},
		{"0b1010", int64(10)},
		{"1_000_000", int64(1000000)},
		{"1.5", 1.5},
		{"1e3", float64(1000)},
		{"1_000.5", 1000.5},
		{"9223372036854775808", float64(9223372036854775808)},
	}
	for _, c := range cases {
		toks := lexAll(t, "<?php "+c.src)
		got, err := numLit(toks[0])
		if err != nil {
			t.Errorf("%q: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %#v, want %#v", c.src, got, c.want)
		}
	}
}
