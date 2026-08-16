package parser_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
)

// tokTriple extracts (id, text) from an array-form token, or (-1, str) from a
// single-char string token.
func tokTriple(t *testing.T, v any) (int, string) {
	t.Helper()
	if s, ok := v.(string); ok {
		return -1, s
	}
	tok, ok := v.([]any)
	if !ok {
		t.Fatalf("token is neither string nor []any: %T", v)
	}
	if len(tok) != 3 {
		t.Fatalf("token has %d elements, want 3: %v", len(tok), tok)
	}
	return int(tok[0].(int64)), tok[1].(string)
}

func TestTokenGetAllShape(t *testing.T) {
	// Mirrors how minitpl wraps an expression before tokenizing.
	toks := parser.TokenGetAll(`<?php if ($this->_vars) { ?>`)

	var got []struct {
		id   int
		text string
	}
	for _, v := range toks {
		id, text := tokTriple(t, v)
		got = append(got, struct {
			id   int
			text string
		}{id, text})
	}

	// Verify key tokens are classified the way minitpl's _split_exp expects.
	var sawOpenTag, sawIf, sawVar, sawArrow bool
	for _, g := range got {
		switch {
		case g.id == parser.T_OPEN_TAG:
			sawOpenTag = true
		case g.id == parser.T_IF && g.text == "if":
			sawIf = true
		case g.id == parser.T_VARIABLE && g.text == "$this":
			sawVar = true
		case g.id == parser.T_OBJECT_OPERATOR && g.text == "->":
			sawArrow = true
		}
	}
	if !sawOpenTag || !sawIf || !sawVar || !sawArrow {
		t.Fatalf("missing expected tokens: open=%v if=%v var=%v arrow=%v\n%+v",
			sawOpenTag, sawIf, sawVar, sawArrow, got)
	}
}

// TestTokenTripleIsIsolated guards the chunked backing array: each triple must
// report cap 3 so a PHP-side append cannot scribble over the next token.
func TestTokenTripleIsIsolated(t *testing.T) {
	toks := parser.TokenGetAll("<?php $a = 1;\n$b = 2;\n$c = 3;\n")
	var triples int
	for _, v := range toks {
		tok, ok := v.([]any)
		if !ok {
			continue
		}
		triples++
		if len(tok) != 3 || cap(tok) != 3 {
			t.Fatalf("triple %v has len %d cap %d, want 3/3", tok, len(tok), cap(tok))
		}
	}
	if triples == 0 {
		t.Fatal("no array-form tokens produced")
	}
}

// TestTokenLineNumbers checks that lines beyond the runtime's interned-int
// range (0..255) still box correctly through boxInt.
func TestTokenLineNumbers(t *testing.T) {
	src := "<?php\n" + strings.Repeat("$a;\n", 1500) + "$last;"
	toks := parser.TokenGetAll(src)
	var lastLine int
	for _, v := range toks {
		tok, ok := v.([]any)
		if !ok {
			continue
		}
		if tok[0].(int64) == int64(parser.T_VARIABLE) && tok[1].(string) == "$last" {
			lastLine = int(tok[2].(int64))
		}
	}
	if lastLine != 1502 {
		t.Fatalf("$last reported on line %d, want 1502", lastLine)
	}
}

func TestIfConditionSyntax(t *testing.T) {
	if _, err := parser.Parse(`<?php $foo = false; if !$foo { echo "no"; }`); err != nil {
		t.Fatalf("unwrapped if condition failed to parse: %v", err)
	}

	if _, err := parser.Parse(`<?php if ($row = fn()) { echo "ok"; }`); err != nil {
		t.Fatalf("assignment-in-condition should parse for runtime/lint compatibility: %v", err)
	}
}

func TestTokenSingleCharStrings(t *testing.T) {
	toks := parser.TokenGetAll(`<?php ($a);`)
	var parens int
	for _, v := range toks {
		if s, ok := v.(string); ok && (s == "(" || s == ")" || s == ";") {
			parens++
		}
	}
	if parens != 3 {
		t.Fatalf("expected 3 single-char tokens, got %d", parens)
	}
}

func TestTokenName(t *testing.T) {
	cases := map[int]string{
		parser.T_VARIABLE:        "T_VARIABLE",
		parser.T_OBJECT_OPERATOR: "T_OBJECT_OPERATOR",
		parser.T_DOUBLE_COLON:    "T_PAAMAYIM_NEKUDOTAYIM",
		parser.T_IF:              "T_IF",
		-1:                       "UNKNOWN",
	}
	for id, want := range cases {
		if got := parser.TokenName(id); got != want {
			t.Fatalf("TokenName(%d) = %q, want %q", id, got, want)
		}
	}
}

func TestTokenVariableWithDotMarker(t *testing.T) {
	// minitpl replaces "." with "__1" before tokenizing so nested accessors stay
	// part of one T_VARIABLE; verify that holds.
	toks := parser.TokenGetAll(`<?php $foo__1bar`)
	var found string
	for _, v := range toks {
		if tok, ok := v.([]any); ok {
			if int(tok[0].(int64)) == parser.T_VARIABLE {
				found = tok[1].(string)
			}
		}
	}
	if found != "$foo__1bar" {
		t.Fatalf("got %q, want $foo__1bar", found)
	}
}

// benchTemplate is a minitpl-shaped template: inline HTML interleaved with
// short PHP expressions, which is what the compiler tokenizes on every include.
const benchTemplate = `<html>
<head><title><?php echo $this->_vars['title']; ?></title></head>
<body>
<?php if ($this->_vars['user']) { ?>
	<p>Hello, <?php echo $this->_vars['user']['name']; ?>!</p>
<?php } else { ?>
	<p>Please <a href="/login">sign in</a>.</p>
<?php } ?>
<ul>
<?php foreach ($this->_vars['items'] as $k => $item) { ?>
	<li id="item-<?php echo $k; ?>">
		<span class="name"><?php echo $item['name']; ?></span>
		<span class="price"><?php echo number_format($item['price'], 2); ?></span>
	</li>
<?php } ?>
</ul>
<!-- a comment -->
<?php echo $this->_render_footer(); // trailing line comment ?>
</body>
</html>
`

func BenchmarkTokenGetAllTemplate(b *testing.B) {
	src := strings.Repeat(benchTemplate, 8)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		if toks := parser.TokenGetAll(src); len(toks) == 0 {
			b.Fatal("no tokens")
		}
	}
}

// BenchmarkTokenGetAllExpr measures the hot minitpl path: one wrapped
// expression tokenized per template tag.
func BenchmarkTokenGetAllExpr(b *testing.B) {
	const src = `<?php if ($this->_vars__1user__1name) { ?>`
	b.ReportAllocs()
	for b.Loop() {
		if toks := parser.TokenGetAll(src); len(toks) == 0 {
			b.Fatal("no tokens")
		}
	}
}
