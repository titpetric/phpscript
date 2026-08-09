package tests

import (
	"fmt"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
)

// FuzzFlatstackImportSwapFallback differentially checks native flat bytecode
// against runner with bounded arithmetic programs. The historical name remains
// stable so existing fuzz corpora and CI commands keep working.
func FuzzFlatstackImportSwapFallback(f *testing.F) {
	f.Add(int64(20), int64(22), uint8(0))
	f.Add(int64(-5), int64(9), uint8(1))
	f.Add(int64(7), int64(6), uint8(2))

	f.Fuzz(func(t *testing.T, left, right int64, operation uint8) {
		operators := [...]string{"+", "-", "*"}
		op := operators[int(operation)%len(operators)]
		source := fmt.Sprintf("<?php echo %d %s %d; ?>", left, op, right)
		program, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := flatstack.Supports(program); err != nil {
			t.Fatalf("arithmetic left native flat bytecode: %v", err)
		}

		var runnerOutput strings.Builder
		runnerRuntime := runner.New(&runnerOutput, runner.Options{})
		runnerErr := runnerRuntime.Run(program)

		var flatOutput strings.Builder
		flatRuntime := flatstack.New(&flatOutput, flatstack.Options{})
		flatErr := flatRuntime.Run(program)

		if (runnerErr == nil) != (flatErr == nil) {
			t.Fatalf("error mismatch: runner=%v flatstack=%v", runnerErr, flatErr)
		}
		if runnerOutput.String() != flatOutput.String() {
			t.Fatalf("output mismatch: runner=%q flatstack=%q", runnerOutput.String(), flatOutput.String())
		}
	})
}
