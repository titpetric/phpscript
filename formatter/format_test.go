package formatter_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/formatter"
	"github.com/titpetric/phpscript/parser"
)

func TestSourceOTBSTabsAndFunction(t *testing.T) {
	in := `<?php

class Test
{
	fn isValid() {
		echo "Test OK";
	}


	func isAdmin()
	{
		return false;
	}
}
`
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "fn ") || strings.Contains(out, "func ") {
		t.Fatalf("fn/func not expanded:\n%s", out)
	}
	if !strings.Contains(out, "function isValid()") {
		t.Fatalf("missing function keyword:\n%s", out)
	}
	if strings.Contains(out, "\n{") {
		t.Fatalf("Allman braces remain:\n%s", out)
	}
	if !strings.Contains(out, "class Test {") {
		t.Fatalf("expected OTBS class brace:\n%s", out)
	}
	if strings.Contains(out, "    ") {
		t.Fatalf("spaces used for indent:\n%s", out)
	}
	if !strings.Contains(out, "\tfunction isValid()") {
		t.Fatalf("expected tab indent:\n%s", out)
	}
	if strings.Contains(out, "\n\n\n") {
		t.Fatalf("double blank lines remain:\n%s", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatal("missing trailing newline")
	}
}

func TestASTRoundTripParses(t *testing.T) {
	in := `<?php
namespace App;

class Database {
	protected $handle;

	public function connect($name) {
		$this->handle = new \PS\Database($name);
		if (!is_array($name)) {
			$index = 0;
		}
		foreach ($values as $k => $v) {
			$query .= "$k";
		}
	}
}
`
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(out); err != nil {
		t.Fatalf("formatted output does not parse: %v\n%s", err, out)
	}
	for _, want := range []string{
		"namespace App;",
		"protected $handle;",
		"public function connect($name) {",
		"new \\PS\\Database($name)",
		"if (!is_array($name)) {",
		"foreach ($values as $k => $v) {",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestLeadingRouteCommentsPreserved(t *testing.T) {
	in := `<?php
// @route GET /kv/{key}
$shm = new SharedMemory;
echo $shm->get($_PATH["key"]);
`
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "// @route GET /kv/{key}") {
		t.Fatalf("route comment lost:\n%s", out)
	}
}

func TestDropCloseTagForClassOnlyFile(t *testing.T) {
	in := "<?php\nclass Foo\n{\n}\n?>\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "?>") {
		t.Fatalf("?> not removed:\n%s", out)
	}
}

func TestIdempotent(t *testing.T) {
	in := `<?php
class Test {
	fn isValid() {
		echo "ok";
	}
}
`
	once, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := formatter.Source(once)
	if err != nil {
		t.Fatal(err)
	}
	if once != twice {
		t.Fatalf("not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
}

func TestPrintUsesProgramNamespace(t *testing.T) {
	prog, err := parser.Parse(`<?php
namespace Fixture;
class Loaded {
	var $source;
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if prog.Namespace != "Fixture" {
		t.Fatalf("namespace = %q", prog.Namespace)
	}
	out := formatter.Print(prog, formatter.Options{})
	if !strings.Contains(out, "namespace Fixture;") {
		t.Fatalf("printed:\n%s", out)
	}
	if !strings.Contains(out, "class Loaded {") {
		t.Fatalf("printed:\n%s", out)
	}
}
