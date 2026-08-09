package flatstack_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
)

// FuzzFlatstackCompilerInput is adapted from the source flat-stack compiler
// fuzz target. Parse and unsupported errors are valid; panics are not.
func FuzzFlatstackCompilerInput(f *testing.F) {
	seeds := []string{
		`<?php $a = "value"; echo $a; ?>`,
		`<?php $a = "left"; $b = "right"; echo $a . $b; ?>`,
		`before <?php echo "inside"; ?> after`,
		`<?php $storage = new Storage; $storage->tenant(); ?>`,
		`<?php echo 10 + 20; ?>`,
		`<?php $a = "unterminated; ?>`,
		`<?php if ($a) { echo "fallback"; } ?>`,
		`<?php`,
		``,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 16<<10 {
			t.Skip()
		}
		program, err := parser.Parse(source)
		if err != nil {
			return
		}
		// Compilation is the target. Arbitrary input can now produce supported,
		// intentionally infinite loops, so executing it would make fuzzing hang.
		_ = flatstack.Supports(program)
	})
}

// FuzzFlatstackDifferential compares supported bytecode with runner semantics.
// Hex encoding keeps arbitrary bytes safe inside a PHP string literal.
func FuzzFlatstackDifferential(f *testing.F) {
	f.Add([]byte("hello"), []byte("world"))
	f.Add([]byte{0, 1, 2, 255}, []byte("suffix"))
	f.Add([]byte{}, []byte{})

	f.Fuzz(func(t *testing.T, leftBytes, rightBytes []byte) {
		if len(leftBytes)+len(rightBytes) > 8<<10 {
			t.Skip()
		}
		left := hex.EncodeToString(leftBytes)
		right := hex.EncodeToString(rightBytes)
		source := `<?php $left = "` + left + `"; $right = "` + right + `"; echo $left . ":" . $right; ?>`
		program, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := flatstack.Supports(program); err != nil {
			t.Fatalf("generated program left flat subset: %v", err)
		}

		var flatOutput strings.Builder
		flatRuntime := flatstack.New(&flatOutput, flatstack.Options{})
		if err := flatRuntime.Run(program); err != nil {
			t.Fatal(err)
		}

		var runnerOutput strings.Builder
		runnerRuntime := runner.New(&runnerOutput, runner.Options{})
		if err := runnerRuntime.Run(program); err != nil {
			t.Fatal(err)
		}
		if flatOutput.String() != runnerOutput.String() {
			t.Fatalf("output mismatch: flatstack=%q runner=%q", flatOutput.String(), runnerOutput.String())
		}
		if want := left + ":" + right; flatOutput.String() != want {
			t.Fatalf("output = %q, want %q", flatOutput.String(), want)
		}
	})
}

// FuzzFlatstackModelAST probes compiler boundaries that source parsing cannot
// produce, including nil and partially initialized nodes.
func FuzzFlatstackModelAST(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5})
	f.Add([]byte{})
	f.Add([]byte{255, 255, 255})

	f.Fuzz(func(t *testing.T, shape []byte) {
		if len(shape) > 4096 {
			t.Skip()
		}
		program := &model.Program{}
		for _, selector := range shape {
			switch selector % 7 {
			case 0:
				program.Stmts = append(program.Stmts, nil)
			case 1:
				program.Stmts = append(program.Stmts, &model.Echo{Args: []model.Expr{nil}})
			case 2:
				program.Stmts = append(program.Stmts, &model.Assign{})
			case 3:
				program.Stmts = append(program.Stmts, &model.ExprStmt{X: &model.Binary{Op: "."}})
			case 4:
				program.Stmts = append(program.Stmts, &model.InlineHTML{Text: "safe"})
			case 5:
				program.Stmts = append(program.Stmts, &model.Echo{Args: []model.Expr{&model.Lit{Value: int64(selector)}}})
			case 6:
				program.Stmts = append(program.Stmts, &model.Return{})
			}
		}
		_ = flatstack.Supports(program)
	})
}
