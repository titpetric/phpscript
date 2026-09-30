package flatstack_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/tests"
)

// benchmarkEngines runs one program through the interpreter and through flat
// bytecode as engine= sub-benchmarks, so the two columns line up under
// `benchstat -col /engine`. The program is gated on Supports: both engines
// must run it natively or the comparison measures fallback by accident.
func benchmarkEngines(b *testing.B, source string, register func(*runner.Runtime)) {
	program := mustFlatProgram(b, source)

	run := func(rt *runner.Runtime) func(*testing.B) {
		return func(b *testing.B) {
			stdlib.Register(rt)
			if register != nil {
				register(rt)
			}
			if err := rt.Run(program); err != nil { // warm caches
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if err := rt.Run(program); err != nil {
					b.Fatal(err)
				}
			}
		}
	}

	b.Run("engine=runner", run(runner.New(io.Discard, runner.Options{})))
	b.Run("engine=flatstack", run(flatstack.New(io.Discard, flatstack.Options{})))
}

func BenchmarkEngineConcat(b *testing.B) {
	benchmarkEngines(b, `<?php $a = "hello"; $b = "-world"; echo $a . $b; ?>`, nil)
}

func BenchmarkEngineLanguageLoop(b *testing.B) {
	benchmarkEngines(b, `<?php
$sum = 0;
for ($i = 0; $i < 20; $i += 1) {
    if ($i % 2 == 0) $sum += $i;
}
echo $sum;
?>`, nil)
}

// BenchmarkEngineExprHeavy is the engine-comparison companion to the
// end-to-end BenchmarkScriptExprHeavy in tests: a loop body that is almost
// entirely expression evaluation, so the two engines' expression paths carry
// the whole number.
func BenchmarkEngineExprHeavy(b *testing.B) {
	benchmarkEngines(b, `<?php
$total = 0;
$tag = "";
for ($i = 0; $i < 100; $i++) {
	$total = ($total + $i * 3) % 97;
	$tag = $total > 48 ? "hi" : "lo";
	if ($tag === "hi") {
		$total = $total + strlen($tag . $i);
	}
}
echo $total, " ", $tag;
`, nil)
}

// BenchmarkEngineUserFunc measures the per-call cost of a PHP function frame:
// push, parameter bind, body, return-value pop.
func BenchmarkEngineUserFunc(b *testing.B) {
	benchmarkEngines(b, `<?php
function twice($x) {
	return $x * 2 + 1;
}
$sum = 0;
for ($i = 0; $i < 50; $i++) {
	$sum = $sum + twice($i);
}
echo $sum;
`, nil)
}

func BenchmarkEngineHostBridge(b *testing.B) {
	benchmarkEngines(b, `<?php
$storage = new Storage;
$storage->set("color", "blue");
$record = $storage->get("color");
echo $storage->tenant() . ":" . $record->value;
`, func(rt *runner.Runtime) {
		rt.RegisterConstructor("Storage", tests.NewStorage)
	})
}
