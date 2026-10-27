package formatter_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/formatter"
	"github.com/titpetric/phpscript/parser"
)

func TestSourceOTBSTabsAndFunction(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	in := `<?php
namespace App;

class Database {
	protected $handle;

	public function connect($name) {
		$this->handle = new \Host\Database($name);
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
		"new \\Host\\Database($name)",
		"if (!is_array($name)) {",
		"foreach ($values as $k => $v) {",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestLeadingRouteCommentsPreserved(t *testing.T) {
	t.Parallel()
	in := `<?php
// @route GET /kv/{key}
$shm = new SharedMemory;
echo $shm->get($_REQUEST["key"]);
`
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "// @route GET /kv/{key}") {
		t.Fatalf("route comment lost:\n%s", out)
	}
}

func TestTrailingCommentStaysOnItsStatement(t *testing.T) {
	t.Parallel()
	in := "<?php\n$value = 1; // value docs\nfunction helper() {}\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "$value = 1; // value docs") {
		t.Fatalf("trailing comment was moved off its statement:\n%s", out)
	}
}

func TestDeclarationCommentIsNotDuplicatedOnSameLine(t *testing.T) {
	t.Parallel()
	in := "<?php\n// first only\nfunction first() {} function second() {}\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "// first only") != 1 {
		t.Fatalf("declaration comment duplicated:\n%s", out)
	}
}

func TestTemplateStartingWithInlineHTMLIsUnchanged(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

	results, err := formatter.Paths([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	changed := formatter.Changed(results)
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

// A directory of PHP holds code phpscript does not support yet. Formatting the
// rest serves an author better than stopping at the first file that does not
// parse and the formatter reports an unreadable file and leaves it untouched.
func TestPathsSkipsFilesItCannotFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.php")
	script := filepath.Join(dir, "script.php")
	brokenSource := "<?php\nclass Test {\n"
	if err := os.WriteFile(broken, []byte(brokenSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("<?php\necho  \"body\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := formatter.Paths([]string{dir})
	if err != nil {
		t.Fatalf("a file that does not parse must not fail the run: %v", err)
	}
	changed := formatter.Changed(results)
	if len(changed) != 1 || changed[0] != script {
		t.Fatalf("changed = %v, want [%s]", changed, script)
	}
	var skipped []string
	for _, result := range results {
		if result.Skipped != nil {
			skipped = append(skipped, result.Path)
		}
	}
	if len(skipped) != 1 || skipped[0] != broken {
		t.Fatalf("skipped = %v, want [%s]", skipped, broken)
	}
	after, err := os.ReadFile(broken)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != brokenSource {
		t.Fatalf("skipped file was rewritten: %q", after)
	}
}

func TestNeedFormattingLeavesFilesAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	script := filepath.Join(dir, "script.php")
	source := "<?php\necho  \"body\";\n"
	if err := os.WriteFile(script, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := formatter.NeedFormatting([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if changed := formatter.Changed(results); len(changed) != 1 || changed[0] != script {
		t.Fatalf("changed = %v, want [%s]", changed, script)
	}
	after, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != source {
		t.Fatalf("listing rewrote the file: %q", after)
	}
}

func TestShebangScriptIsFormattedAndKeepsItsInterpreterLine(t *testing.T) {
	t.Parallel()
	in := "#!/usr/bin/env phpscript\n<?php\necho  \"hello\";\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	want := "#!/usr/bin/env phpscript\n<?php\n\necho \"hello\";\n"
	if out != want {
		t.Fatalf("unexpected output:\n--- got ---\n%s--- want ---\n%s", out, want)
	}
}

func TestSourceNormalizesLineEndings(t *testing.T) {
	t.Parallel()
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

func TestIdempotent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

var update = flag.Bool("update", false, "rewrite testdata golden files")

// TestGolden formats every testdata/*.php input and compares it against the
// matching .golden file. Each case is also checked for the two properties the
// formatter has to hold when it rewrites a file in place: the output parses,
// and formatting it again does not change it.
func TestGolden(t *testing.T) {
	t.Parallel()
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.php"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata inputs")
	}
	for _, input := range inputs {
		t.Run(strings.TrimSuffix(filepath.Base(input), ".php"), func(t *testing.T) {
			src, err := os.ReadFile(input)
			if err != nil {
				t.Fatal(err)
			}
			out, err := formatter.Source(string(src))
			if err != nil {
				t.Fatalf("format: %v", err)
			}
			if _, err := parser.Parse(out); err != nil {
				t.Fatalf("formatted output does not parse: %v\n%s", err, out)
			}
			again, err := formatter.Source(out)
			if err != nil {
				t.Fatalf("reformat: %v", err)
			}
			if again != out {
				t.Fatalf("formatting is not idempotent:\n--- first ---\n%s--- second ---\n%s", out, again)
			}

			golden := strings.TrimSuffix(input, ".php") + ".golden"
			if *update {
				if err := os.WriteFile(golden, []byte(out), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if out != string(want) {
				t.Fatalf("output differs from %s:\n--- got ---\n%s--- want ---\n%s", golden, out, want)
			}
		})
	}
}
