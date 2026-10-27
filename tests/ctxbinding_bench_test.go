package tests_test

import (
	"context"
	"io"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// BenchmarkScriptContextBinding prices the context a binding receives: one
// hundred calls of a registered function whose first parameter declares a
// context.Context and the per-call cost of building that context is the
// benchmark's slope and the session setup is its floor. runner.bindingContext
// is the site it prices.
func BenchmarkScriptContextBinding(b *testing.B) {
	prog, err := parser.Parse(`<?php
$n = 0;
for ($i = 0; $i < 100; $i++) {
	$n = bench_ctx();
}
echo $n;`)
	if err != nil {
		b.Fatal(err)
	}

	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)
	rt.RegisterFunc("bench_ctx", func(ctx context.Context) int64 {
		if ctx == nil {
			return 0
		}
		return 1
	})

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rt.ResetSession(io.Discard, nil)
		if err := rt.Run(prog); err != nil {
			b.Fatal(err)
		}
	}
}
