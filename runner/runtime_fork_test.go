package runner_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
)

// forkParent is a runtime that has run src, so it holds the declarations a fork
// carries over.
func forkParent(t *testing.T, src string) (*runner.Runtime, *strings.Builder) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return rt, &out
}

// TestRuntime_Fork covers what a fork carries and what it does not. It carries
// the symbols the tree declared, so the same function is callable on the child,
// and none of the execution: a fork is safe on a goroutine of its own, which is
// the whole reason it exists.
func TestRuntime_Fork(t *testing.T) {
	t.Parallel()
	parent, _ := forkParent(t, `<?php function twice($n) { return $n * 2; } $x = 7;`)

	var childOut strings.Builder
	child := parent.Fork(&childOut)
	if child == parent {
		t.Fatal("Fork answered the runtime it was called on")
	}

	got, err := child.InvokeNamed("twice", int64(21))
	if err != nil {
		t.Fatalf("the declaration did not reach the fork: %v", err)
	}
	if got != int64(42) {
		t.Errorf("twice(21) on the fork = %v, want 42", got)
	}

	// $x is execution state and stays with the parent.
	if _, err := child.InvokeNamed("missing", nil); err == nil {
		t.Error("a name nothing declares resolved on the fork")
	}
}

// TestRuntime_DeclaredPrograms covers the table a host replays onto forks it
// builds itself. Fork does it already, so this is for a host managing its own.
//
// It answers a copy, so a caller walking it while the runtime includes another
// file is not reading the map the include writes.
func TestRuntime_DeclaredPrograms(t *testing.T) {
	t.Parallel()
	parent, _ := forkParent(t, `<?php function once() { return 1; }`)

	programs := parent.DeclaredPrograms()
	if len(programs) == 0 {
		t.Fatal("DeclaredPrograms() answered nothing for a runtime that ran a program")
	}

	var held *model.Program
	for program := range programs {
		held = program
	}
	delete(programs, held)
	if len(parent.DeclaredPrograms()) == len(programs) {
		t.Error("deleting from the answer changed the runtime's own table")
	}
}

// TestRuntime_SetPreparer covers how a fork gets its bindings. A binding is
// almost always a closure over its registering runtime, so a copied
// entry would read and write the parent from inside the child; the preparer is
// the host saying how to install them again.
func TestRuntime_SetPreparer(t *testing.T) {
	t.Parallel()
	parent, _ := forkParent(t, `<?php $noop = 1;`)

	installed := 0
	parent.SetPreparer(func(rt *runner.Runtime) {
		installed++
		rt.RegisterFunc("host_answer", func() string { return "installed" })
	})

	var out strings.Builder
	child := parent.Fork(&out)
	if installed == 0 {
		t.Fatal("Fork did not run the preparer")
	}

	got, err := child.InvokeNamed("host_answer")
	if err != nil {
		t.Fatalf("the preparer's binding is not callable on the fork: %v", err)
	}
	if got != "installed" {
		t.Errorf("host_answer() = %v, want \"installed\"", got)
	}
}

// TestRuntime_InvokeClosure covers the re-entry a host callback needs. The
// declaration is AST, so it runs on whichever runtime the caller holds, and it
// gets a scope holding its arguments and nothing else.
//
// The declaration comes off the parse tree and not out of a script variable:
// a *model.Closure is what the method takes, and that is the form a host
// holding one has.
func TestRuntime_InvokeClosure(t *testing.T) {
	t.Parallel()
	prog, err := parser.Parse(`<?php $f = function ($n) { return $n + 1; };`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var decl *model.Closure
	for _, stmt := range prog.Stmts {
		expr, ok := stmt.(*model.Assign)
		if !ok {
			continue
		}
		if closure, ok := expr.Value.(*model.Closure); ok {
			decl = closure
		}
	}
	if decl == nil {
		t.Fatal("no closure declaration in the parse tree")
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})

	got, err := rt.InvokeClosure(decl, int64(41))
	if err != nil {
		t.Fatalf("InvokeClosure: %v", err)
	}
	if got != int64(42) {
		t.Errorf("the closure answered %v, want 42", got)
	}

	if _, err := rt.InvokeClosure(nil); err == nil {
		t.Error("InvokeClosure(nil) answered no error")
	}
}
