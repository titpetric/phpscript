package runner_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// benchCalls runs body in a loop of 100, so a per-call figure is the reported
// one less the baseline, divided by a hundred.
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

// BenchmarkDispatchReflect calls one it does not. str_repeat is
// func(string, int64) string, which is not among the shapes, so the call goes
// through reflectInvoker and reflect.Value.Call.
//
// The pair is what a new shape in fastInvoker is worth. 163 of the 314
// registered functions take the reflect path, across 126 distinct signatures,
// so the question is per function rather than a sweep.
func BenchmarkDispatchReflect(b *testing.B) {
	benchCalls(b, `str_repeat("a", 3);`)
}

// BenchmarkDispatchBaseline is the loop with no call in it.
func BenchmarkDispatchBaseline(b *testing.B) {
	benchCalls(b, ``)
}
