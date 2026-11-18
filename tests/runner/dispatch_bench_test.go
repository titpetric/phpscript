package runner_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// benchCalls runs body in a loop of 100. A per-call figure is the reported one
// minus the baseline, divided by a hundred.
func benchCalls(b *testing.B, body string) {
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)
	rt.FreezeStdlib()
	program, err := rt.Load(`<?php for ($i = 0; $i < 100; $i++) { ` + body + ` }`)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := rt.Run(program); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDispatchFast calls a binding whose signature fastInvoker answers
// directly. strtoupper is func(string) string, one of the shapes it lists.
func BenchmarkDispatchFast(b *testing.B) {
	benchCalls(b, `strtoupper("abc");`)
}

// BenchmarkDispatchReflect calls a function fastInvoker does not. str_repeat is
// func(string, int64) string, which is not among the shapes. reflectInvoker
// dispatches the call through reflect.Value.Call.
//
// The pair measures a new shape's worth in fastInvoker. 163 of the 314
// registered functions take the reflect path, across 126 distinct signatures.
// Each function decides separately, without a sweep.
func BenchmarkDispatchReflect(b *testing.B) {
	benchCalls(b, `str_repeat("a", 3);`)
}

// BenchmarkDispatchBaseline is the loop holding no call.
func BenchmarkDispatchBaseline(b *testing.B) {
	benchCalls(b, ``)
}

// BenchmarkDispatchSprintf prices one of the string family's most called
// names. sprintf is func(string, ...any) (string, error): the error-returning
// spelling of a shape fastInvoker already lists without the error. The
// figure moves with only which invoker the registration built.
//
// It exists because the shape of a binding's return is not a local decision.
// Adding an error return is a correctness change that reads as a single line.
// It decides the dispatch path for every later call to that name. This
// benchmark prices sprintf. The move lands measured.
func BenchmarkDispatchSprintf(b *testing.B) {
	benchCalls(b, `sprintf("%s-%d", "a", 3);`)
}
