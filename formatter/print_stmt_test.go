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

func TestPHPReopensForClosingBraceAfterInlineHTML(t *testing.T) {
	t.Parallel()
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
