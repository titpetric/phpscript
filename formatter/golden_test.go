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

var update = flag.Bool("update", false, "rewrite testdata golden files")

// TestGolden formats every testdata/*.php input and compares it against the
// matching .golden file. Each case is also checked for the two properties the
// formatter has to hold when it rewrites a file in place: the output parses,
// and formatting it again does not change it.
func TestGolden(t *testing.T) {
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
