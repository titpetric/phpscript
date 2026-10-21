package formatter_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/formatter"
)

func TestFunctionsAndStatementsKeepOpeningBraceOnSameLine(t *testing.T) {
	t.Parallel()
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

func TestWhitespaceBetweenStatements(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
