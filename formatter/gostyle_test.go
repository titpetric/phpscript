package formatter

import (
	"testing"
)

// TestGoStyleLine covers the trailing semicolon and the member spelling.
func TestGoStyleLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{`$server = new Server;`, `$server = new Server`},
		{`import "net/http";`, `import "net/http"`},
		{`$r->URL->Query()->Get("name");`, `$r->url->query()->get("name")`},
		{`return;`, `return`},
		{`break;`, `break`},
		{`function hello() {`, `function hello() {`},
		{`} while ($x);`, `} while ($x);`},
		{`});`, `})`},
		{`public $name;`, `public $name;`},
		{`echo "a;->b";`, `echo "a;->b"`},
		{`$s = "x;";`, `$s = "x;"`},
		{`$x = 1; // note`, `$x = 1 // note`},
		{`$x = 1; /* note */`, `$x = 1 /* note */`},
	}
	for _, test := range tests {
		var scan styleScan
		if got := scan.line(test.in); got != test.want {
			t.Errorf("line(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

// TestGoStyleKeepsBlockComment covers a block comment that spans lines with a semicolon inside.
func TestGoStyleKeepsBlockComment(t *testing.T) {
	t.Parallel()
	var scan styleScan
	if got := scan.line(`/* a;`); got != `/* a;` {
		t.Fatalf("first line = %q", got)
	}
	if got := scan.line(`b; */`); got != `b; */` {
		t.Fatalf("second line = %q", got)
	}
}

// TestIsGoRuntimeSource covers the two spellings a template file writes.
func TestIsGoRuntimeSource(t *testing.T) {
	t.Parallel()
	if !isGoRuntimeSource("<?php\nimport \"net/http\";\n") {
		t.Error("an import did not mark the file")
	}
	if !isGoRuntimeSource("<?php\nuse Runtime\\Go;\n") {
		t.Error("a use did not mark the file")
	}
	if isGoRuntimeSource("<?php\necho \"hello\";\n") {
		t.Error("a php file was marked")
	}
}
