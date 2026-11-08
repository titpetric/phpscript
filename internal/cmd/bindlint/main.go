// Command bindlint reports a binding that points at a runtime value type:
// a parameter declared as *model.Array or *model.Object, and a binding that
// narrows a script value to one of them for no reason the code states. Both
// make a binding answer for the values the interpreter built and for no
// others, and the same value from another binding is a Go slice, map or
// struct. It exits 1 when there is any.
//
//	go run ./internal/cmd/bindlint [dir]
//
// The rules and the reasoning behind them are in
// docs/allocation-performance.md; atkins lint runs this beside splint.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run lints the directory args name, the current one by default, and returns
// the exit code: 1 with findings, 2 when the walk fails.
func run(args []string, stdout, stderr io.Writer) int {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	findings, err := lint(root)
	if err != nil {
		fmt.Fprintln(stderr, "bindlint:", err)
		return 2
	}
	for _, f := range findings {
		fmt.Fprintln(stdout, f)
	}
	if len(findings) > 0 {
		fmt.Fprintf(stdout, "\n%d bindings reach one shape of a script value\n", len(findings))
		return 1
	}
	return 0
}
