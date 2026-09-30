package flatstack_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/stdlib"
)

// TestFlatstackBitwiseOperators proves the bitwise operators compile to
// bytecode rather than reaching the interpreter fallback. A fixture cannot tell
// the two apart when they agree, so every case asserts Supports first.
func TestFlatstackBitwiseOperators(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "binary operators",
			source: `<?php echo 1 | 2, ";", 6 & 3, ";", 5 ^ 3, ";", 1 << 3, ";", 6 >> 1;`,
			want:   "3;2;6;8;3",
		},
		{
			name:   "unary complement",
			source: `<?php echo ~5, ";", ~-6;`,
			want:   "-6;5",
		},
		{
			name:   "precedence against comparison and addition",
			source: `<?php echo 1 | 2 == 2, ";", 1 << 2 + 3;`,
			want:   "1;32",
		},
		{
			name:   "compound assignment",
			source: `<?php $x = 6; $x &= 3; $x |= 4; $x ^= 3; $x <<= 4; $x >>= 2; echo $x;`,
			want:   "20",
		},
		{
			name:   "bytewise string operands",
			source: `<?php echo "a" | "b", ";", ~ ~"abc";`,
			want:   "c;abc",
		},
		{
			name:   "a negative shift count is catchable",
			source: `<?php try { echo 1 << -1; } catch (ArithmeticError $e) { echo $e->getMessage(); }`,
			want:   "Bit shift by negative number",
		},
		{
			name:   "a negative shift count in a compound assignment is catchable",
			source: `<?php $x = 1; try { $x >>= -1; } catch (ArithmeticError $e) { echo $e->getMessage(); }`,
			want:   "Bit shift by negative number",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := parser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if err := flatstack.Supports(program); err != nil {
				t.Fatalf("expected native bytecode support: %v", err)
			}
			var output strings.Builder
			runtime := flatstack.New(&output, flatstack.Options{})
			// ArithmeticError is a stdlib class registration; without it the
			// catch clause has no name to match.
			stdlib.Register(runtime)
			if err := runtime.Run(program); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}
