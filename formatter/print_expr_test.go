package formatter_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/formatter"
	"github.com/titpetric/phpscript/model"
)

func TestQuoteStyleOfStringLiteralsIsKept(t *testing.T) {
	t.Parallel()
	in := "<?php\n$html = '<span class=\"ok\">OK</span>';\n$row = \"row $index\";\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`$html = '<span class="ok">OK</span>';`,
		`$row = "row $index";`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

// A literal a caller builds without parsing has no source spelling to keep,
// and the printer picks the quoting: single quotes when they avoid escaping a
// dollar sign, which a double-quoted literal would interpolate.
func TestSyntheticStringLiteralAvoidsInterpolation(t *testing.T) {
	t.Parallel()
	prog := &model.Program{Stmts: []model.Stmt{
		&model.Echo{Args: []model.Expr{&model.Lit{Value: `$var "quoted"`}}},
	}}
	out := formatter.Print(prog, formatter.Options{})
	if !strings.Contains(out, `echo '$var "quoted"';`) {
		t.Fatalf("unexpected quoting:\n%s", out)
	}
}

func TestFirstClassCallableSyntaxPreserved(t *testing.T) {
	t.Parallel()
	in := `<?php
$fn = greet(...);
$len = strlen(...);
$shout = Greeter::shout(...);
$dynamicStatic = Greeter::$pick(...);
$method = $obj->method(...);
$dynamicMethod = $obj->$pick(...);
$braced = $obj->{$pick}(...);
$again = $closure(...);
$indexed = $handlers[0](...);
`
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`$fn = greet(...);`,
		`$len = strlen(...);`,
		`$shout = Greeter::shout(...);`,
		`$dynamicStatic = Greeter::$pick(...);`,
		`$method = $obj->method(...);`,
		`$dynamicMethod = $obj->$pick(...);`,
		`$braced = $obj->$pick(...);`,
		`$again = $closure(...);`,
		`$indexed = $handlers[0](...);`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("first-class callable syntax %q changed:\n%s", want, out)
		}
	}
}

func TestFirstClassCallableIsNotACall(t *testing.T) {
	t.Parallel()
	// A zero-argument call and the callable that names it print differently,
	// the node carries the spelling for that reason.
	out, err := formatter.Source("<?php\n$a = greet();\n$b = greet(...);\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "$a = greet();") || !strings.Contains(out, "$b = greet(...);") {
		t.Fatalf("call and callable did not stay distinct:\n%s", out)
	}
}

// A go builtin a file imported resolves to a binding under the go namespace and
// prints as the source wrote it: the qualified name is the one the runtime
// looks up and the spelling is the one a reader sees, so a file that uses go's
// builtins is still a round trip through the formatter.
func TestGoBuiltinCallKeepsItsSpelling(t *testing.T) {
	t.Parallel()
	in := "<?php\n\nuse Runtime\\Go;\n\n$s = make([]int64, 0, 10);\n$s = append($s, int64($n));\necho len($s), cap($s);\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("formatting a go builtin changed the source:\n%s\nwant:\n%s", out, in)
	}
}
