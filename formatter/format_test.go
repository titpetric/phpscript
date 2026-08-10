package formatter_test

import (
	"os"
	"path/filepath"
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
	if strings.Contains(out, "function isAdmin()\n") {
		t.Fatalf("function opening brace moved to next line:\n%s", out)
	}
	if !strings.Contains(out, "class Test {") {
		t.Fatalf("expected class brace on declaration line:\n%s", out)
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
		"class Database {",
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

func TestFunctionsAndStatementsKeepOpeningBraceOnSameLine(t *testing.T) {
	in := `<?php
function check($values)
{
	if ($values)
		echo "yes";
	foreach ($values as $value)
		echo $value;
	while ($values)
		break;
}
`
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"function check($values) {",
		"if ($values) {",
		"foreach ($values as $value) {",
		"while ($values) {",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestBareExitRemainsBare(t *testing.T) {
	out, err := formatter.Source("<?php\nexit;\ndie;\nexit();\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "exit;\ndie;\nexit();") {
		t.Fatalf("exit syntax changed:\n%s", out)
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

func TestClosingTagPreservedAroundInlineHTML(t *testing.T) {
	in := "<?php\nif ($ok) { ?>\n<p>ok</p>\n<?php echo \"done\";\n}\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "if ($ok) {\n\n\t?>\n<p>ok</p>\n<?php\n\techo \"done\";") {
		t.Fatalf("PHP/HTML transitions lost:\n%s", out)
	}
	if _, err := parser.Parse(out); err != nil {
		t.Fatalf("formatted mixed PHP/HTML does not parse: %v\n%s", err, out)
	}
}

func TestPHPReopensForClosingBraceAfterInlineHTML(t *testing.T) {
	in := "<?php\nif ($ok) { ?>inline<?php }\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "inline<?php\n}") {
		t.Fatalf("closing brace emitted outside PHP:\n%s", out)
	}
	if _, err := parser.Parse(out); err != nil {
		t.Fatalf("formatted output does not parse: %v\n%s", err, out)
	}
	again, err := formatter.Source(out)
	if err != nil || again != out {
		t.Fatalf("mixed-content output is not idempotent: %v\nfirst: %q\nsecond: %q", err, out, again)
	}
}

func TestTemplateStartingWithInlineHTMLIsUnchanged(t *testing.T) {
	in := "<h1>Title</h1>\n<?php echo \"body\";\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("template changed: %q", out)
	}
}

func TestPathsReturnsChangedFilesAndSkipsTemplates(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "script.php")
	template := filepath.Join(dir, "template.php")
	templateSource := "<h1>Title</h1>\n<?php echo  \"body\";\n"
	if err := os.WriteFile(script, []byte("<?php\necho  \"body\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(template, []byte(templateSource), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := formatter.ChangedPaths([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != script {
		t.Fatalf("changed = %v, want [%s]", changed, script)
	}
	templateAfter, err := os.ReadFile(template)
	if err != nil {
		t.Fatal(err)
	}
	if string(templateAfter) != templateSource {
		t.Fatalf("template changed: %q", templateAfter)
	}
}

func TestSourceNormalizesLineEndings(t *testing.T) {
	in := "<?php\r\necho \"a\r\nb\";\r\n?>\r\n<p>done</p>\r\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\r") {
		t.Fatalf("non-LF line ending remains: %q", out)
	}
	if !strings.Contains(out, `echo "a\r\nb";`) {
		t.Fatalf("string literal value changed: %q", out)
	}
	if !strings.Contains(out, "?>\n<p>done</p>\n") {
		t.Fatalf("unexpected normalized output: %q", out)
	}
}

func TestIncludeAndRequireSyntaxPreserved(t *testing.T) {
	in := `<?php
include "a.php";
include_once("b.php");
require "c.php";
require_once("d.php");
$result = require("e.php");
$other = include "f.php";
`
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`include "a.php";`,
		`include_once("b.php");`,
		`require "c.php";`,
		`require_once("d.php");`,
		`$result = require("e.php");`,
		`$other = include "f.php";`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("include/require syntax %q changed:\n%s", want, out)
		}
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

func TestWhitespaceBetweenStatements(t *testing.T) {
	in := "<?php\nfunction run() {\n\n\n\t$a = 1;   \n\t$b = 2;\n\n\n\t$c = 3;\n\twork();\n\tif ($a) {\n\t\techo $a;\n\t}\n\tafter();\n\tif ($b) {\n\t\techo $b;\n\t}\n\n\n}\n"
	want := "<?php\n\nfunction run() {\n\t$a = 1;\n\t$b = 2;\n\n\t$c = 3;\n\n\twork();\n\tif ($a) {\n\t\techo $a;\n\t}\n\n\tafter();\n\tif ($b) {\n\t\techo $b;\n\t}\n}\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Fatalf("unexpected whitespace:\n--- got ---\n%s--- want ---\n%s", out, want)
	}
	again, err := formatter.Source(out)
	if err != nil || again != out {
		t.Fatalf("output is not idempotent: %v\nfirst: %q\nsecond: %q", err, out, again)
	}
}

func TestClassMethodAndClosingTagWhitespace(t *testing.T) {
	in := "<?php\nclass Example\n{\n\n\tfunction first() {\n\n\t\techo \"first\";\n\n\t}\n\n\n\tfunction second() {\n\t\techo \"second\";\n\t}\n}\nafter_class();\n?>\n<p>html</p>\n"
	want := "<?php\n\nclass Example {\n\tfunction first() {\n\t\techo \"first\";\n\t}\n\n\tfunction second() {\n\t\techo \"second\";\n\t}\n}\nafter_class();\n\n?>\n<p>html</p>\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Fatalf("unexpected class/tag whitespace:\n--- got ---\n%s--- want ---\n%s", out, want)
	}
}

func TestNoForcedBlankAfterClass(t *testing.T) {
	in := "<?php\nclass First {}\nfunction next() {}\nclass Second {}\nclass Third {}\n"
	want := "<?php\n\nclass First {\n}\nfunction next() {\n}\n\nclass Second {\n}\nclass Third {\n}\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Fatalf("unexpected declaration spacing:\n--- got ---\n%s--- want ---\n%s", out, want)
	}
}

func TestTerminalClosingTagAndWhitespaceTrimmedIdempotently(t *testing.T) {
	in := "<?php\r\necho \"x\";\r\n?>\r\n \t\r\n"
	want := "<?php\n\necho \"x\";\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Fatalf("terminal tag remains:\n--- got ---\n%q\n--- want ---\n%q", out, want)
	}
	again, err := formatter.Source(out)
	if err != nil || again != out {
		t.Fatalf("output is not idempotent: %v\nfirst: %q\nsecond: %q", err, out, again)
	}
}

func TestTrailingWhitespaceTrimmedFromMixedHTML(t *testing.T) {
	in := "<?php\r\necho \"x\";\r\n?>\r\n<p>x</p>  \r\n"
	want := "<?php\n\necho \"x\";\n\n?>\n<p>x</p>\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Fatalf("trailing whitespace remains:\n--- got ---\n%q\n--- want ---\n%q", out, want)
	}
}
