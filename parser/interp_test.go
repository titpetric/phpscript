package parser

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// parseInterpExpr parses src as one echo argument and returns the expression.
func parseInterpExpr(t *testing.T, src string) model.Expr {
	t.Helper()
	prog, err := Parse("<?php echo " + src + ";")
	if err != nil {
		t.Fatalf("parse %s: %v", src, err)
	}
	echo, ok := prog.Stmts[0].(*model.Echo)
	if !ok {
		t.Fatalf("parse %s: got %T, want *model.Echo", src, prog.Stmts[0])
	}
	return echo.Args[0]
}

// TestLiteralWithoutInterpolationStaysALit guards the fast path: a literal that
// embeds nothing must still parse to a plain Lit, whichever quote it uses and
// whether or not it carries an escape or a dollar the escape kept literal.
func TestLiteralWithoutInterpolationStaysALit(t *testing.T) {
	for _, src := range []string{
		`"plain"`,
		`"with \n escape"`,
		`"\$name"`,
		`"$ name"`,
		`"trailing $"`,
		`'single $name'`,
		`'single {$name}'`,
		`""`,
	} {
		if _, ok := parseInterpExpr(t, src).(*model.Lit); !ok {
			t.Errorf("%s: got %T, want *model.Lit", src, parseInterpExpr(t, src))
		}
	}
}

// TestInterpolationPartShapes checks that each spelling lowers to the AST the
// runtime expects, in particular that a bare word subscript becomes a string
// key rather than a constant reference.
func TestInterpolationPartShapes(t *testing.T) {
	tests := []struct {
		src   string
		parts []string
	}{
		{`"$name"`, []string{"var:name"}},
		{`"a$name"`, []string{"lit:a", "var:name"}},
		{`"$a$b"`, []string{"var:a", "var:b"}},
		{`"$row[id]"`, []string{"index:row:id"}},
		{`"$row[0]"`, []string{"index:row:0"}},
		{`"$row[$i]"`, []string{"indexvar:row:i"}},
		{`"$obj->prop"`, []string{"prop:obj:prop"}},
		{`"$obj->prop->more"`, []string{"prop:obj:prop", "lit:->more"}},
		{`"{$name}"`, []string{"var:name"}},
		{`"x{$name}y"`, []string{"lit:x", "var:name", "lit:y"}},
	}

	for _, tt := range tests {
		e := parseInterpExpr(t, tt.src)
		interp, ok := e.(*model.Interp)
		if !ok {
			t.Errorf("%s: got %T, want *model.Interp", tt.src, e)
			continue
		}
		got := make([]string, 0, len(interp.Parts))
		for _, part := range interp.Parts {
			got = append(got, describePart(part))
		}
		if strings.Join(got, "|") != strings.Join(tt.parts, "|") {
			t.Errorf("%s: got %v, want %v", tt.src, got, tt.parts)
		}
	}
}

func describePart(e model.Expr) string {
	switch n := e.(type) {
	case *model.Lit:
		s, _ := n.Value.(string)
		return "lit:" + s
	case *model.Var:
		return "var:" + n.Name
	case *model.PropAccess:
		base, _ := n.Base.(*model.Var)
		return "prop:" + base.Name + ":" + n.Name
	case *model.Index:
		base, _ := n.Base.(*model.Var)
		switch key := n.Index.(type) {
		case *model.Lit:
			switch v := key.Value.(type) {
			case string:
				return "index:" + base.Name + ":" + v
			case int64:
				return "index:" + base.Name + ":" + string(rune('0'+v))
			}
		case *model.Var:
			return "indexvar:" + base.Name + ":" + key.Name
		}
	}
	return "?"
}

// TestInterpolationKeepsSourceSpelling is what the formatter relies on: it
// prints the literal back from Raw rather than re-encoding it, so a rewrite
// cannot change which spelling of an embedded expression the author chose.
func TestInterpolationKeepsSourceSpelling(t *testing.T) {
	for _, src := range []string{`"a $b c"`, `"{$a['k']}"`, `"$a[k]\n"`} {
		e := parseInterpExpr(t, src)
		interp, ok := e.(*model.Interp)
		if !ok {
			t.Fatalf("%s: got %T, want *model.Interp", src, e)
		}
		if interp.Raw != src {
			t.Errorf("%s: Raw is %q, want the source spelling", src, interp.Raw)
		}
	}
}

// TestInterpolationErrors covers the spellings that are reported rather than
// guessed at. Each one would otherwise produce a literal the author did not
// write, which is the failure worth stopping.
func TestInterpolationErrors(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`"${name}"`, "${...} string interpolation is not supported"},
		{`"$a[k"`, "unterminated ["},
		{`"{$a[0]"`, "unterminated string in {$...}"},
		{`"$a[k()]"`, "is not a simple subscript"},
	}
	for _, tt := range tests {
		_, err := Parse("<?php echo " + tt.src + ";")
		if err == nil {
			t.Errorf("%s: parsed, want an error", tt.src)
			continue
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: got %q, want it to mention %q", tt.src, err, tt.want)
		}
	}
}

// TestInterpolationLineNumbers checks that a literal spanning lines does not
// desynchronise the line counter for what follows it.
func TestInterpolationLineNumbers(t *testing.T) {
	toks := lexAll(t, "<?php\n$a = \"x $b\ny $c\";\n$d = 1;\n")
	var last token
	for _, tok := range toks {
		if tok.kind == tVar && tok.val == "d" {
			last = tok
		}
	}
	if last.line != 4 {
		t.Errorf("$d reported on line %d, want 4", last.line)
	}
}
