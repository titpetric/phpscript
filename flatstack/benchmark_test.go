package flatstack_test

import (
	"context"
	"io"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
)

func mustFlatProgram(tb testing.TB, source string) *model.Program {
	tb.Helper()
	program, err := parser.Parse(source)
	if err != nil {
		tb.Fatal(err)
	}
	if err := flatstack.Supports(program); err != nil {
		tb.Fatalf("benchmark would use interpreter fallback: %v", err)
	}
	return program
}

// Benchmarks are modeled after the flat-stack repository's compile-once/run-
// many and parallel-load matrix. They deliberately gate on Supports.
func BenchmarkFlatstackPrecompiledConcat(b *testing.B) {
	program := mustFlatProgram(b, `<?php $a = "hello"; $b = "-world"; echo $a . $b; ?>`)
	runtime := flatstack.New(io.Discard, flatstack.Options{})
	if err := runtime.Run(program); err != nil { // warm bytecode cache
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := runtime.Run(program); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFlatstackPrecompiledLanguageLoop(b *testing.B) {
	program := mustFlatProgram(b, `<?php
$sum = 0;
for ($i = 0; $i < 20; $i += 1) {
    if ($i % 2 == 0) $sum += $i;
}
echo $sum;
?>`)
	runtime := flatstack.New(io.Discard, flatstack.Options{})
	if err := runtime.Run(program); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := runtime.Run(program); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFlatstackParseCompileRun(b *testing.B) {
	const source = `<?php $a = "hello"; $b = "-world"; echo $a . $b; ?>`
	b.ReportAllocs()
	for b.Loop() {
		program, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		runtime := flatstack.New(io.Discard, flatstack.Options{})
		if err := runtime.Run(program); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFlatstackParallelHostBridge(b *testing.B) {
	program := mustFlatProgram(b, `<?php
$storage = new Storage;
$storage->set("color", "blue");
$record = $storage->get("color");
echo $storage->tenant() . ":" . $record->value;
`)
	cache := flatstack.NewExprCache()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		runtime := flatstack.New(io.Discard, flatstack.Options{})
		runtime.SetExprCache(cache)
		runtime.SetContext(context.WithValue(context.Background(), contextKey("tenant"), "acme"))
		runtime.RegisterConstructor("Storage", newStorage)
		for pb.Next() {
			if err := runtime.Run(program); err != nil {
				b.Fatal(err)
			}
		}
	})
}
