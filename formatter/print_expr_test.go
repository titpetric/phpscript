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
