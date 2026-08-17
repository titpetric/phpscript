package compat_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// runScript runs src against a runtime with the stdlib installed and returns
// what reached the real output. The bindings are exercised through the VM
// because that is where buffering has to hold up: it works by intercepting what
// echo and inline HTML write, not by anything a script can observe directly.
func runScript(t *testing.T, src string) string {
	t.Helper()
	program, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// TestOutputBufferingIsWiredByImport covers the blank import: stdlib installs
// the ob_* family without naming this package, so a script can call it.
func TestOutputBufferingIsWiredByImport(t *testing.T) {
	for _, test := range []struct {
		name string
		php  string
		want string
	}{
		{
			name: "captured output does not reach the writer",
			php:  `<?php echo "before;"; ob_start(); echo "captured;"; ob_end_clean(); echo "after";`,
			want: "before;after",
		},
		{
			name: "ob_get_clean hands the text back",
			php:  `<?php ob_start(); echo "captured"; echo "[" . ob_get_clean() . "]";`,
			want: "[captured]",
		},
		{
			name: "ob_get_contents leaves the buffer open",
			php:  `<?php ob_start(); echo "text"; $seen = ob_get_contents(); $rest = ob_get_clean(); echo $seen . "/" . $rest;`,
			want: "text/text",
		},
		{
			name: "buffers nest",
			php:  `<?php ob_start(); echo "outer("; ob_start(); echo "inner"; echo ob_get_clean() . ")"; echo ob_get_clean();`,
			want: "outer(inner)",
		},
		{
			name: "ob_end_flush writes to the enclosing level",
			php:  `<?php ob_start(); echo "kept;"; ob_end_flush(); echo "then";`,
			want: "kept;then",
		},
		{
			name: "ob_get_flush both returns and writes",
			php:  `<?php ob_start(); echo "text;"; $seen = ob_get_flush(); echo "seen=" . $seen;`,
			want: "text;seen=text;",
		},
		{
			name: "levels are counted",
			php:  `<?php echo ob_get_level(); ob_start(); ob_start(); $n = ob_get_level(); ob_end_clean(); ob_end_clean(); echo $n . ob_get_level();`,
			want: "020",
		},
		{
			name: "inline HTML is captured too",
			php:  `<?php ob_start(); ?>markup<?php echo "[" . ob_get_clean() . "]";`,
			want: "[markup]",
		},
		{
			// PHP distinguishes "no buffer" from "an empty buffer", and a
			// script tells them apart with ===.
			name: "no active buffer reports false",
			php:  `<?php echo ob_get_contents() === false ? "false" : "string"; ob_start(); echo ob_get_clean() === "" ? ";empty" : ";other";`,
			want: "false;empty",
		},
		{
			name: "ending a buffer that is not there reports false",
			php:  `<?php echo ob_end_clean() ? "ended" : "none"; echo ob_get_clean() === false ? ";false" : ";string";`,
			want: "none;false",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := runScript(t, test.php); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}

// TestBuffersAreScopedToOneRuntime pins what lets concurrent requests buffer at
// the same time: the stack belongs to the runtime the bindings were installed
// on, not to the package.
func TestBuffersAreScopedToOneRuntime(t *testing.T) {
	program, err := parser.Parse(`<?php ob_start(); echo "captured"; $text = ob_get_clean(); echo "[" . $text . "]" . ob_get_level();`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var first, second strings.Builder
	firstRT := runner.New(&first, runner.Options{})
	secondRT := runner.New(&second, runner.Options{})
	stdlib.Register(firstRT)
	stdlib.Register(secondRT)

	// The first runtime is left mid-capture, so a shared stack would leak into
	// the second one's level count and swallow its output.
	firstRT.PushOutput(&strings.Builder{})

	if err := secondRT.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := second.String(); got != "[captured]0" {
		t.Fatalf("output = %q, want %q", got, "[captured]0")
	}
	if got := first.String(); got != "" {
		t.Fatalf("first runtime wrote %q, want nothing", got)
	}
}
