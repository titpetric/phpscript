package formatter_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/formatter"
	"github.com/titpetric/phpscript/parser"
)

func TestBareExitRemainsBare(t *testing.T) {
	t.Parallel()
	out, err := formatter.Source("<?php\nexit;\ndie;\nexit();\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "exit;\ndie;\nexit();") {
		t.Fatalf("exit syntax changed:\n%s", out)
	}
}

func TestExplicitExpressionParenthesesRetained(t *testing.T) {
	t.Parallel()
	in := "<?php\n$offset = ($page - 1) * $limit;\n$value = (($a + $b));\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"$offset = ($page - 1) * $limit;",
		"$value = (($a + $b));",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expression grouping %q changed:\n%s", want, out)
		}
	}
	again, err := formatter.Source(out)
	if err != nil || again != out {
		t.Fatalf("output is not idempotent: %v\nfirst: %q\nsecond: %q", err, out, again)
	}
}

func TestClosingTagPreservedAroundInlineHTML(t *testing.T) {
	t.Parallel()
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

// TestInlinePHPBlockIsKept covers a line that opens and closes PHP: the tags,
// the code between and the text around them are template markup. The line
// stays as written.
func TestInlinePHPBlockIsKept(t *testing.T) {
	t.Parallel()
	in := "<?php\nif ($ok) { ?>inline<?php }\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("an inline PHP block changed:\n%q\nwant:\n%q", out, in)
	}
}

// TestTemplateInlineBraceNestsLines covers the three-line template: the
// `foreach` on the first line nests the line between, and the closing line
// aligns with the one that opened the brace.
func TestTemplateInlineBraceNestsLines(t *testing.T) {
	t.Parallel()
	in := "<?php foreach ($items as $item) { ?>\n    <li><?php echo $item; ?></li>\n<?php } ?>\n"
	want := "<?php foreach ($items as $item) { ?>\n\t<li><?php echo $item; ?></li>\n<?php } ?>\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Fatalf("template lines formatted as:\n%q\nwant:\n%q", out, want)
	}
}

// TestAssignmentAndCallStayTogether covers a call written under an assignment:
// the blank line the author left between them comes out.
func TestAssignmentAndCallStayTogether(t *testing.T) {
	t.Parallel()
	in := "<?php\n$server = new Server;\n\n$server->run();\n"
	want := "<?php\n\n$server = new Server;\n$server->run();\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Fatalf("assignment and call formatted as:\n%q\nwant:\n%q", out, want)
	}
}

func TestIncludeAndRequireSyntaxPreserved(t *testing.T) {
	t.Parallel()
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

// TestGoRuntimeTemplateTakesGoStyle covers a go runtime file the aligner
// handles: code lines take the go spelling, inline template lines do not.
func TestGoRuntimeTemplateTakesGoStyle(t *testing.T) {
	t.Parallel()
	in := "<?php\nimport \"net/http\"\n\nclass Page {\n\tfunction show(http.ResponseWriter $w, *http.Request $r): void {\n\t\t$r->URL->Path;\n\t\t?>\n\t\t<p><?php echo $r->PathValue(\"id\"); ?></p>\n\t\t<?php\n\t}\n}\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"import \"net/http\"\n",
		"$r->url->path\n",
		"<p><?php echo $r->PathValue(\"id\"); ?></p>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
