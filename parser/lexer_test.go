package parser

import (
	"os"
	"path/filepath"
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
		{"**", []string{"*", "*"}},
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
		{tVar, "a", 2}, {tOp, "->", 2}, {tIdent, "b", 2}, {tOp, ";", 2},
		{tVar, "c", 3}, {tOp, "[", 3}, {tInt, "1", 3}, {tOp, "]", 3},
		{tOp, "=", 3}, {tInt, "2", 3}, {tOp, ";", 3},
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

// fixtureSource reads a PHP file from the repository fixtures. The fixtures are
// read-only inputs for the parser benchmarks.
func fixtureSource(tb testing.TB, name string) string {
	tb.Helper()
	return readSource(tb, filepath.Join("..", "tests", "fixtures", "code", name))
}

// engineSource reads a file of the titpetric/minitpl sources, the parser's
// largest real-world input. It comes from composer rather than a checked-in
// copy, so a tree where `composer install` has not run skips these cases.
func engineSource(tb testing.TB, name string) string {
	tb.Helper()
	return readSource(tb, filepath.Join("..", "tests", "fixtures", "vendor", "titpetric", "minitpl", "code", "MiniTPL", name))
}

func readSource(tb testing.TB, path string) string {
	tb.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		tb.Skipf("fixture %s unavailable: %v", path, err)
	}
	return string(b)
}

func BenchmarkLexCompiler(b *testing.B) {
	src := engineSource(b, "Compiler.php")
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := newLexer(src).run(); err != nil {
			b.Fatal(err)
		}
	}
}
