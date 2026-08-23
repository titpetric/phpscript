package parser

import (
	"fmt"
	"strings"
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

func TestParseClassModifiers(t *testing.T) {
	cases := []struct {
		src                         string
		abstract, final, isReadonly bool
	}{
		{`<?php class A {}`, false, false, false},
		{`<?php abstract class A {}`, true, false, false},
		{`<?php final class A {}`, false, true, false},
		{`<?php readonly class A {}`, false, false, true},
		{`<?php final readonly class A {}`, false, true, true},
		{`<?php readonly final class A {}`, false, true, true},
		{`<?php abstract readonly class A {}`, true, false, true},
	}
	for _, tc := range cases {
		prog := mustParse(t, tc.src)
		cd, ok := prog.Stmts[0].(*model.ClassDecl)
		if !ok {
			t.Fatalf("%s: got %T, want *model.ClassDecl", tc.src, prog.Stmts[0])
		}
		if cd.Abstract != tc.abstract || cd.Final != tc.final || cd.Readonly != tc.isReadonly {
			t.Errorf("%s: abstract/final/readonly = %v/%v/%v, want %v/%v/%v",
				tc.src, cd.Abstract, cd.Final, cd.Readonly, tc.abstract, tc.final, tc.isReadonly)
		}
	}
}

// A namespaced file may only declare symbols, so a modifier the parser does not
// recognise surfaces there as a rejected file rather than a dropped keyword.
func TestParseClassModifiersInNamespace(t *testing.T) {
	prog := mustParse(t, "<?php\nnamespace App;\nfinal readonly class Thing {}\n")
	cd, ok := prog.Stmts[len(prog.Stmts)-1].(*model.ClassDecl)
	if !ok {
		t.Fatalf("got %T, want *model.ClassDecl", prog.Stmts[len(prog.Stmts)-1])
	}
	if cd.Name != `App\Thing` || !cd.Final || !cd.Readonly {
		t.Errorf("class = %q final=%v readonly=%v, want App\\Thing final readonly", cd.Name, cd.Final, cd.Readonly)
	}
}

func TestParseClassModifiersRejected(t *testing.T) {
	cases := map[string]string{
		"abstract final class A {}": "final modifier on an abstract class",
		"final abstract class A {}": "final modifier on an abstract class",
		"final final class A {}":    "multiple final modifiers",
		"final function f() {}":     "expected class after class modifiers",
	}
	for src, want := range cases {
		if _, err := Parse("<?php " + src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want one containing %q", src, err, want)
		}
	}
}

// `readonly` is not a reserved word in PHP; it is a modifier only when a class
// declaration follows it.
func TestParseReadonlyAsCallable(t *testing.T) {
	prog := mustParse(t, `<?php readonly(3);`)
	call, ok := prog.Stmts[0].(*model.ExprStmt).X.(*model.Call)
	if !ok || call.Name != "readonly" {
		t.Fatalf("got %#v, want Call(readonly)", prog.Stmts[0])
	}
}

// `extends` and `implements` are recorded on the AST but confer no
// inheritance; the parser's only job is to keep the names.
func TestParseClassHeritage(t *testing.T) {
	prog := mustParse(t, "<?php\nnamespace App;\n\nuse Vendor\\Framework\\TestCase;\n\nfinal class Suite extends TestCase implements \\Countable, Local {}\n")
	cd, ok := prog.Stmts[len(prog.Stmts)-1].(*model.ClassDecl)
	if !ok {
		t.Fatalf("got %T, want *model.ClassDecl", prog.Stmts[len(prog.Stmts)-1])
	}
	if cd.Parent != `Vendor\Framework\TestCase` {
		t.Errorf("parent = %q, want Vendor\\Framework\\TestCase", cd.Parent)
	}
	want := []string{"Countable", `App\Local`}
	if len(cd.Implements) != len(want) {
		t.Fatalf("implements = %v, want %v", cd.Implements, want)
	}
	for i, name := range want {
		if cd.Implements[i] != name {
			t.Errorf("implements[%d] = %q, want %q", i, cd.Implements[i], name)
		}
	}
}

func TestParseClassHeritageOmitted(t *testing.T) {
	cd := mustParse(t, `<?php class Plain {}`).Stmts[0].(*model.ClassDecl)
	if cd.Parent != "" || cd.Implements != nil {
		t.Errorf("parent/implements = %q/%v, want empty", cd.Parent, cd.Implements)
	}
}

func TestParseClassHeritageRejected(t *testing.T) {
	cases := map[string]string{
		"class A extends {}":      "expected class name after extends",
		"class A implements {}":   "expected interface name after implements",
		"class A implements B, {": "expected interface name after implements",
	}
	for src, want := range cases {
		if _, err := Parse("<?php " + src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want one containing %q", src, err, want)
		}
	}
}

// The restriction on a namespaced file is deliberate, so the message has to
// carry the reason and the way out, and point at the offending statement
// rather than at whatever follows it.
func TestParseNamespacedStatementRejected(t *testing.T) {
	_, err := Parse("<?php\nnamespace App;\n\nclass Thing {}\n\necho \"hi\";\n")
	if err == nil {
		t.Fatal("a top-level statement in a namespaced file must be rejected")
	}
	for _, want := range []string{
		"line 6:",
		"may only declare classes and functions",
		"scanned for the symbols it declares at include time",
		"move this statement into a function",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// `use` and `declare` are preamble rather than code, so a namespaced file may
// still carry them.
func TestParseNamespacedPreambleAllowed(t *testing.T) {
	src := "<?php\ndeclare(strict_types=1);\n\nnamespace App;\n\nuse Vendor\\Thing;\n\nfunction f() {\n\techo \"hi\";\n}\n"
	if _, err := Parse(src); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// exprShape renders an expression fully parenthesised, so a precedence test
// reads as the tree the parser built rather than as a chain of type
// assertions.
func exprShape(e model.Expr) string {
	switch n := e.(type) {
	case *model.Binary:
		return "(" + exprShape(n.Left) + " " + n.Op + " " + exprShape(n.Right) + ")"
	case *model.Unary:
		if n.Postfix {
			return "(" + exprShape(n.X) + n.Op + ")"
		}
		return "(" + n.Op + exprShape(n.X) + ")"
	case *model.Parenthesized:
		return exprShape(n.X)
	case *model.Lit:
		return fmt.Sprintf("%v", n.Value)
	case *model.Var:
		return "$" + n.Name
	default:
		return fmt.Sprintf("%T", e)
	}
}

// TestParseBitwisePrecedence pins where the bitwise operators sit in PHP 8's
// precedence table: `| ^ &` are looser than any comparison and tighter than
// `&&`, and `<< >>` are tighter than `.` and looser than `+ -`. Every
// expectation is what the php binary evaluates the same source to.
func TestParseBitwisePrecedence(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"1 | 2 == 2", "(1 | (2 == 2))"},
		{"7 & 3 == 3", "(7 & (3 == 3))"},
		{"1 < 2 & 3", "((1 < 2) & 3)"},
		{"1 ^ 2 & 3", "(1 ^ (2 & 3))"},
		{"2 | 4 ^ 6", "(2 | (4 ^ 6))"},
		{"6 & 3 | 1 ^ 2", "((6 & 3) | (1 ^ 2))"},
		{"$a && $b | $c", "($a && ($b | $c))"},
		{"$a || $b & $c", "($a || ($b & $c))"},
		{"1 << 2 + 3", "(1 << (2 + 3))"},
		{`"a" . 1 << 2`, "(a . (1 << 2))"},
		{"1 << 2 << 3", "((1 << 2) << 3)"},
		{"16 >> 1 >> 2", "((16 >> 1) >> 2)"},
		{"1 | 2 | 3", "((1 | 2) | 3)"},
		{"~$a & $b", "((~$a) & $b)"},
		{"~ ~5", "(~(~5))"},
		{"-$a >> 1", "((-$a) >> 1)"},
		{"$a & $b == $c", "($a & ($b == $c))"},
		{"(1 | 2) == 2", "((1 | 2) == 2)"},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			prog := mustParse(t, "<?php $r = "+test.src+";")
			assign, ok := prog.Stmts[0].(*model.Assign)
			if !ok {
				t.Fatalf("got %T, want *model.Assign", prog.Stmts[0])
			}
			if got := exprShape(assign.Value); got != test.want {
				t.Errorf("%s parses as %s, want %s", test.src, got, test.want)
			}
		})
	}
}

// The by-reference marker used to be skipped in front of any operand, which
// swallowed the binary `&` of `echo 6 & 3` and printed 6. It is a marker only
// in front of a variable now, and nothing else may consume an operator
// silently.
func TestParseAmpersandIsNotSwallowed(t *testing.T) {
	prog := mustParse(t, `<?php echo 6 & 3;`)
	echo, ok := prog.Stmts[0].(*model.Echo)
	if !ok {
		t.Fatalf("got %T, want *model.Echo", prog.Stmts[0])
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("got %d statements, want 1: %#v", len(prog.Stmts), prog.Stmts)
	}
	if len(echo.Args) != 1 {
		t.Fatalf("got %d echo args, want 1", len(echo.Args))
	}
	if got := exprShape(echo.Args[0]); got != "(6 & 3)" {
		t.Fatalf("echo arg = %s, want (6 & 3)", got)
	}
	if _, err := Parse(`<?php echo & 3;`); err == nil {
		t.Fatal("a reference marker in front of a literal must be rejected")
	}
}

// A reference marker still parses where PHP allows one: in front of a
// variable, which is where every by-reference construct puts it.
func TestParseReferenceMarkerBeforeVariable(t *testing.T) {
	for _, src := range []string{
		`<?php $a = &$b;`,
		`<?php f(&$b);`,
		`<?php foreach ($rows as &$row) { $row = 1; }`,
		`<?php $fn = function () use (&$b) { return $b; };`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("parse %q: %v", src, err)
		}
	}
}
