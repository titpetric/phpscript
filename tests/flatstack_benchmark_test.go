package tests

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
)

// BenchmarkFlatstackMinitplImportSwap quantifies the compatibility-fallback
// cost for a real application fixture that is not yet in the bytecode subset.
func BenchmarkFlatstackMinitplImportSwap(b *testing.B) {
	source, err := fixturesFS.ReadFile("fixtures/test-minitpl.php")
	if err != nil {
		b.Fatal(err)
	}
	program, err := parser.Parse(string(source))
	if err != nil {
		b.Fatal(err)
	}
	if err := flatstack.Supports(program); err == nil {
		b.Fatal("minitpl unexpectedly became a flat-bytecode benchmark; update this benchmark")
	}

	b.ReportAllocs()
	for b.Loop() {
		var output strings.Builder
		runtime := newFlatstackTestRuntime(&output, b.Context())
		if err := runtime.Run(program); err != nil {
			b.Fatal(err)
		}
	}
}
