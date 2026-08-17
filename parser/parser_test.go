package parser

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

func mustParse(t *testing.T, src string) *model.Program {
	t.Helper()
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return prog
}

func TestParseEchoArgs(t *testing.T) {
	prog := mustParse(t, `<?php echo "a", $b, 1 + 2;`)
	if len(prog.Stmts) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Stmts))
	}
	e, ok := prog.Stmts[0].(*model.Echo)
	if !ok {
		t.Fatalf("got %T, want *model.Echo", prog.Stmts[0])
	}
	if len(e.Args) != 3 {
		t.Fatalf("got %d echo args, want 3", len(e.Args))
	}
	if cap(e.Args) != len(e.Args) {
		t.Errorf("echo args cap %d, want exactly %d", cap(e.Args), len(e.Args))
	}
	if lit, ok := e.Args[0].(*model.Lit); !ok || lit.Value != "a" {
		t.Errorf("arg 0 = %#v, want Lit(\"a\")", e.Args[0])
	}
	if v, ok := e.Args[1].(*model.Var); !ok || v.Name != "b" {
		t.Errorf("arg 1 = %#v, want Var(b)", e.Args[1])
	}
	if bin, ok := e.Args[2].(*model.Binary); !ok || bin.Op != "+" {
		t.Errorf("arg 2 = %#v, want Binary(+)", e.Args[2])
	}
}

func TestParseBinaryPrecedence(t *testing.T) {
	prog := mustParse(t, `<?php $r = 1 + 2 * 3 . "x" == $y;`)
	as, ok := prog.Stmts[0].(*model.Assign)
	if !ok {
		t.Fatalf("got %T, want *model.Assign", prog.Stmts[0])
	}
	eq, ok := as.Value.(*model.Binary)
	if !ok || eq.Op != "==" {
		t.Fatalf("top = %#v, want Binary(==)", as.Value)
	}
	concat, ok := eq.Left.(*model.Binary)
	if !ok || concat.Op != "." {
		t.Fatalf("left = %#v, want Binary(.)", eq.Left)
	}
	add, ok := concat.Left.(*model.Binary)
	if !ok || add.Op != "+" {
		t.Fatalf("concat left = %#v, want Binary(+)", concat.Left)
	}
	mul, ok := add.Right.(*model.Binary)
	if !ok || mul.Op != "*" {
		t.Fatalf("add right = %#v, want Binary(*)", add.Right)
	}
}

func TestParsePostfixChain(t *testing.T) {
	prog := mustParse(t, `<?php $a = $this->b[0]->c($d)[1];`)
	as := prog.Stmts[0].(*model.Assign)
	idx, ok := as.Value.(*model.Index)
	if !ok {
		t.Fatalf("got %T, want *model.Index", as.Value)
	}
	call, ok := idx.Base.(*model.MethodCall)
	if !ok || call.Method != "c" {
		t.Fatalf("index base = %#v, want MethodCall(c)", idx.Base)
	}
	prop, ok := call.Base.(*model.Index)
	if !ok {
		t.Fatalf("call base = %#v, want Index", call.Base)
	}
	if pa, ok := prop.Base.(*model.PropAccess); !ok || pa.Name != "b" {
		t.Fatalf("inner = %#v, want PropAccess(b)", prop.Base)
	}
}

// TestParseNodesAreDistinct guards the chunked node allocation: every AST node
// must keep its own address, because runner.compileExpr caches compiled
// expressions in a map keyed by model.Expr pointer identity.
func TestParseNodesAreDistinct(t *testing.T) {
	prog := mustParse(t, `<?php
	$a = $x + $x + $x;
	$b = 1 + 1 + 1;
	f($x, $x, $x);
	$c = $x->y->y->y;
	`)
	seen := map[model.Expr]bool{}
	var walk func(e model.Expr)
	walk = func(e model.Expr) {
		if e == nil {
			return
		}
		if seen[e] {
			t.Fatalf("expression node %#v reused at two positions in the AST", e)
		}
		seen[e] = true
		switch n := e.(type) {
		case *model.Binary:
			walk(n.Left)
			walk(n.Right)
		case *model.Unary:
			walk(n.X)
		case *model.Parenthesized:
			walk(n.X)
		case *model.Index:
			walk(n.Base)
			walk(n.Index)
		case *model.PropAccess:
			walk(n.Base)
		case *model.MethodCall:
			walk(n.Base)
			for _, a := range n.Args {
				walk(a)
			}
		case *model.Call:
			for _, a := range n.Args {
				walk(a)
			}
		}
	}
	for _, s := range prog.Stmts {
		switch n := s.(type) {
		case *model.Assign:
			walk(n.Target)
			walk(n.Value)
		case *model.ExprStmt:
			walk(n.X)
		}
	}
	if len(seen) < 20 {
		t.Fatalf("walked only %d nodes, expected the full tree", len(seen))
	}
}

func TestParseSourceSpansPerStatement(t *testing.T) {
	prog := mustParse(t, "<?php\n$a = 1;\n\n$b =\n  2;\n")
	if len(prog.Stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(prog.Stmts))
	}
	if got := prog.SourceSpans[prog.Stmts[0]]; got.Start != 2 || got.End != 2 {
		t.Errorf("stmt 0 span %+v, want {2 2}", got)
	}
	if got := prog.SourceSpans[prog.Stmts[1]]; got.Start != 4 || got.End != 5 {
		t.Errorf("stmt 1 span %+v, want {4 5}", got)
	}
}

func TestParseStructuralSlicesAreExact(t *testing.T) {
	prog := mustParse(t, `<?php
	function f($a, $b, $c) {
		$x = array(1, 2, 3);
		g($a, $b);
		return $x;
	}
	class K {
		var $one = 1, $two = 2;
		const A = 1, B = 2;
		function m() { return 1; }
	}
	`)
	if got := len(prog.Stmts); got != 2 {
		t.Fatalf("got %d statements, want 2", got)
	}
	if cap(prog.Stmts) != len(prog.Stmts) {
		t.Errorf("program stmts cap %d, want %d", cap(prog.Stmts), len(prog.Stmts))
	}
	fn := prog.Stmts[0].(*model.FuncDecl)
	if len(fn.Params) != 3 || cap(fn.Params) != 3 {
		t.Errorf("params len/cap = %d/%d, want 3/3", len(fn.Params), cap(fn.Params))
	}
	if len(fn.Body) != 3 || cap(fn.Body) != 3 {
		t.Errorf("body len/cap = %d/%d, want 3/3", len(fn.Body), cap(fn.Body))
	}
	call := fn.Body[1].(*model.ExprStmt).X.(*model.Call)
	if len(call.Args) != 2 || cap(call.Args) != 2 {
		t.Errorf("args len/cap = %d/%d, want 2/2", len(call.Args), cap(call.Args))
	}
	arr := fn.Body[0].(*model.Assign).Value.(*model.ArrayLit)
	if len(arr.Items) != 3 || cap(arr.Items) != 3 {
		t.Errorf("array items len/cap = %d/%d, want 3/3", len(arr.Items), cap(arr.Items))
	}
	cls := prog.Stmts[1].(*model.ClassDecl)
	if len(cls.Fields) != 2 {
		t.Errorf("got %d fields, want 2", len(cls.Fields))
	}
	if len(cls.Consts) != 2 {
		t.Errorf("got %d consts, want 2", len(cls.Consts))
	}
	if len(cls.Methods) != 1 {
		t.Errorf("got %d methods, want 1", len(cls.Methods))
	}
}

func TestParseNestedArgumentLists(t *testing.T) {
	prog := mustParse(t, `<?php f(g(1, 2), h(3, i(4, 5), 6), 7);`)
	outer := prog.Stmts[0].(*model.ExprStmt).X.(*model.Call)
	if outer.Name != "f" || len(outer.Args) != 3 {
		t.Fatalf("outer = %s/%d args, want f/3", outer.Name, len(outer.Args))
	}
	g := outer.Args[0].(*model.Call)
	if g.Name != "g" || len(g.Args) != 2 {
		t.Fatalf("g = %s/%d args, want g/2", g.Name, len(g.Args))
	}
	h := outer.Args[1].(*model.Call)
	if h.Name != "h" || len(h.Args) != 3 {
		t.Fatalf("h = %s/%d args, want h/3", h.Name, len(h.Args))
	}
	i := h.Args[1].(*model.Call)
	if i.Name != "i" || len(i.Args) != 2 {
		t.Fatalf("i = %s/%d args, want i/2", i.Name, len(i.Args))
	}
	if lit := outer.Args[2].(*model.Lit); lit.Value != int64(7) {
		t.Errorf("last arg = %#v, want Lit(7)", lit.Value)
	}
}

func TestParseFixtures(t *testing.T) {
	for _, name := range []string{"TestCase.php", "functions.php", "TemplateTest_phpscript.php"} {
		src := fixtureSource(t, name)
		if _, err := Parse(src); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The template engine arrives through composer, so it is named separately.
	for _, name := range []string{"Compiler.php", "Template.php", "Hook.php"} {
		src := engineSource(t, name)
		if _, err := Parse(src); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func BenchmarkParseCompiler(b *testing.B) {
	src := engineSource(b, "Compiler.php")
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseTemplate(b *testing.B) {
	src := engineSource(b, "Template.php")
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse(src); err != nil {
			b.Fatal(err)
		}
	}
}
