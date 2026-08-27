package core_test

import (
	"io"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

func runPHP(t *testing.T, src string) string {
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

// TestNoJSONFlagConstants is the executable form of a design decision: this
// runtime has one JSON encoding and no flags to vary it. If this test fails,
// read docs/design.md before changing it.
//
// The flags exist in PHP because its encoder makes choices a caller then has
// to undo. Escaping a forward slash is the clearest: PHP writes "one\/two",
// which is legal JSON that no other language emits, and JSON_UNESCAPED_SLASHES
// exists to turn it off. Go's encoder does not escape it, so there is nothing
// to turn off and the flag has no work to do here.
//
// The rest are presentation. A consumer that wants indented JSON runs it
// through jq or its own pretty printer; a producer that indents its output is
// deciding for a reader it cannot see.
func TestNoJSONFlagConstants(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)
	for name := range rt.DefinedConstants() {
		if strings.HasPrefix(name, "JSON_") {
			t.Fatalf("%s is registered: json_encode has one encoding and no flags. See docs/design.md.", name)
		}
	}
}

// $flags is accepted and ignored, as json_decode accepts $depth and $flags. A
// port carrying JSON_PRETTY_PRINT runs and encodes: the constant is not
// defined, so it arrives as null and selects nothing. phpscript lint reports
// the name.
func TestJSONEncodeIgnoresFlags(t *testing.T) {
	tests := []string{
		`json_encode(array("a" => 1))`,
		`json_encode(array("a" => 1), 128)`,
		`json_encode(array("a" => 1), JSON_PRETTY_PRINT)`,
		`json_encode(array("a" => 1), JSON_HEX_TAG | JSON_HEX_AMP)`,
	}
	for _, call := range tests {
		t.Run(call, func(t *testing.T) {
			if got := runPHP(t, "<?php echo "+call+";"); got != `{"a":1}` {
				t.Fatalf("got %q, want %q", got, `{"a":1}`)
			}
		})
	}
}

// A forward slash is written as itself, which is Go's behaviour and legal
// JSON. php escapes it unless JSON_UNESCAPED_SLASHES is passed.
func TestJSONEncodeDoesNotEscapeSlashes(t *testing.T) {
	got := runPHP(t, `<?php echo json_encode(array("path" => "a/b"));`)
	if want := `{"path":"a\/b"}`; got == want {
		t.Fatalf("got php's escaped form %q; this runtime writes the slash as itself", got)
	}
	if want := `{"path":"a/b"}`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
