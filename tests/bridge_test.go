package tests_test

// This file measures *dispatch*: what it costs to get from PHP into a Go
// binding and back. bindings_test.go measures the other half, what it costs for
// that binding to return a value, and publishes docs/allocation-performance.md;
// the numbers here publish docs/php-go-calls.md.
//
// Every cell does the same work, one AnalyticsRing.Record, so a difference
// between two cells is the path and nothing else. Each engine cell is run in
// three shapes:
//
//	once        one call in a script, the per-request number
//	loop        the call a thousand times
//	loop_empty  the same loop with an empty body, the control
//
// The marginal cost of one call is loop minus loop_empty. Without the control a
// loop cell measures the engine's loop implementation as much as the bridge,
// which is why it is a cell and not a footnote.

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/tests"
)

// bridgeLoops is how many calls the loop shapes make. Large enough that the
// per-iteration cost dominates the script's own setup, small enough that a
// benchmark iteration stays short.
const bridgeLoops = 1000

// bridgeArgs is the call every cell makes, spelled once so no cell accidentally
// measures a different argument shape.
const bridgeArgs = `"/index.php", 200, 1234`

// bridgeProgram returns the three script shapes for a call spelled as name.
func bridgeProgram(shape, name string) string {
	switch shape {
	case "once":
		return fmt.Sprintf(`<?php %s(%s);`, name, bridgeArgs)
	case "loop":
		return fmt.Sprintf(`<?php for ($i = 0; $i < %d; $i++) { %s(%s); }`, bridgeLoops, name, bridgeArgs)
	case "loop_empty":
		return fmt.Sprintf(`<?php for ($i = 0; $i < %d; $i++) { }`, bridgeLoops)
	}
	panic("unknown shape " + shape)
}

// bridgeCalls is how many bridge crossings a shape performs, used to report
// calls/s. The empty control performs none, so it reports no rate.
func bridgeCalls(shape string) int {
	switch shape {
	case "once":
		return 1
	case "loop":
		return bridgeLoops
	}
	return 0
}

// reportRate adds a calls/s metric, which is the number the POC is asking for.
// A shape that makes no calls reports none rather than zero, so the control
// cell cannot be misread as an infinitely slow bridge.
func reportRate(b *testing.B, shape string) {
	b.Helper()
	calls := bridgeCalls(shape)
	if calls == 0 {
		return
	}
	b.ReportMetric(float64(calls)*float64(b.N)/b.Elapsed().Seconds(), "calls/s")
}

// newBridgeRuntime returns an interpreter runtime with the analytics bindings
// and the standard library, which is what a real host has: the function table
// size is part of what a call costs.
func newBridgeRuntime(out *strings.Builder) *runner.Runtime {
	rt := runner.New(out, runner.Options{})
	stdlib.Register(rt, tests.RegisterAnalytics)
	return rt
}

// newBridgeFlatstack mirrors newBridgeRuntime on the flat bytecode backend.
func newBridgeFlatstack(out *strings.Builder) *flatstack.Runtime {
	rt := flatstack.New(out, flatstack.Options{})
	stdlib.Register(rt, tests.RegisterAnalytics)
	return rt
}

// mustBridgeProgram parses src, and for the flat backend fails when the program
// would fall back to the interpreter. A silent fallback would publish the
// interpreter's number in a flatstack row, so it is a benchmark failure rather
// than a note.
func mustBridgeProgram(tb testing.TB, src string, flat bool) *model.Program {
	tb.Helper()
	program, err := parser.Parse(src)
	if err != nil {
		tb.Fatalf("parse: %v", err)
	}
	if flat {
		if err := flatstack.Supports(program); err != nil {
			tb.Fatalf("benchmark would use interpreter fallback: %v", err)
		}
	}
	return program
}

// benchmarkScriptShape runs one engine cell: compile once, run many, reporting
// allocations and the crossing rate.
func benchmarkScriptShape(b *testing.B, shape, name string, flat bool) {
	b.Helper()
	src := bridgeProgram(shape, name)
	program := mustBridgeProgram(b, src, flat)

	var out strings.Builder
	var run func() error
	if flat {
		rt := newBridgeFlatstack(&out)
		run = func() error { return rt.Run(program) }
	} else {
		rt := newBridgeRuntime(&out)
		run = func() error { return rt.Run(program) }
	}
	if err := run(); err != nil { // warm every cache before the timer starts
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out.Reset()
		if err := run(); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	reportRate(b, shape)
}

// BenchmarkBridge is the whole dispatch matrix. One run produces every cell, so
// benchstat can diff a before and after in a single pass.
func BenchmarkBridge(b *testing.B) {
	ring := tests.NewAnalyticsRing(1024)

	// The floor: no VM, no reflection, just the call the bindings wrap.
	b.Run("direct_go", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			ring.Record("/index.php", 200, 1234)
		}
		b.StopTimer()
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "calls/s")
	})

	// The reflection floor: the sequence runner.invokeAny performs for a
	// binding that is not one of invokeFast's recognised shapes.
	b.Run("direct_reflect", func(b *testing.B) {
		fn := tests.AnalyticsFuncs(ring)["analytics_record"]
		rv := reflect.ValueOf(fn)
		args := []reflect.Value{
			reflect.ValueOf("/index.php"),
			reflect.ValueOf(int64(200)),
			reflect.ValueOf(int64(1234)),
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			rv.Call(args)
		}
		b.StopTimer()
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "calls/s")
	})

	// The pure-expr floor: expr-lang evaluating one call, with none of
	// phpscript's PHP semantics on top. This is what the interpreter cells are
	// measured against, and what a replacement backend has to beat.
	b.Run("expr_raw", func(b *testing.B) {
		// expr hands an untyped integer literal over as int, where phpscript's
		// transpiler emits int64, so this shim coerces the way the runtime's
		// own argument handling would. It is the same work either way.
		env := map[string]any{
			"record": func(args ...any) (any, error) {
				route, _ := args[0].(string)
				status, _ := exprInt(args[1])
				micros, _ := exprInt(args[2])
				ring.Record(route, status, micros)
				return nil, nil
			},
		}
		program, err := expr.Compile(`record("/index.php", 200, 1234)`,
			expr.Env(env), expr.AllowUndefinedVariables(), expr.DisableAllBuiltins())
		if err != nil {
			b.Fatalf("compile: %v", err)
		}
		var out any
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			out, err = vm.Run(program, env)
			if err != nil {
				b.Fatal(err)
			}
		}
		_ = out
		b.StopTimer()
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "calls/s")
	})

	// The engine cells. Each name is the same work reached a different way.
	cells := []struct {
		name string // benchmark cell name
		call string // how the PHP source spells the call
		flat bool
	}{
		{"interp_global_native", "analytics_record", false},
		{"interp_global_uniform", "analytics_record_fast", false},
		{"interp_namespaced", `Analytics\record`, false},
		{"interp_ctx", "analytics_record_ctx", false},
		{"flat_global_native", "analytics_record", true},
		{"flat_global_uniform", "analytics_record_fast", true},
		{"flat_namespaced", `Analytics\record`, true},
		{"flat_ctx", "analytics_record_ctx", true},
	}
	for _, cell := range cells {
		for _, shape := range []string{"once", "loop"} {
			b.Run(cell.name+"/"+shape, func(b *testing.B) {
				benchmarkScriptShape(b, shape, cell.call, cell.flat)
			})
		}
	}

	// The controls. Subtract these from the loop cells to get the marginal cost
	// of one crossing, free of the engine's loop implementation.
	b.Run("interp_control/loop_empty", func(b *testing.B) {
		benchmarkScriptShape(b, "loop_empty", "", false)
	})
	b.Run("flat_control/loop_empty", func(b *testing.B) {
		benchmarkScriptShape(b, "loop_empty", "", true)
	})
}

// BenchmarkBridgeConstruct measures the constructor path, which reaches the
// host through __new and helperNew rather than through a function call.
func BenchmarkBridgeConstruct(b *testing.B) {
	for _, flat := range []bool{false, true} {
		name := "interp_ctor"
		if flat {
			name = "flat_ctor"
		}
		b.Run(name, func(b *testing.B) {
			src := `<?php $b = new Analytics\Buffer(); $b->record("/index.php", 200, 1234);`
			program := mustBridgeProgram(b, src, flat)

			var out strings.Builder
			var run func() error
			if flat {
				rt := newBridgeFlatstack(&out)
				run = func() error { return rt.Run(program) }
			} else {
				rt := newBridgeRuntime(&out)
				run = func() error { return rt.Run(program) }
			}
			if err := run(); err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				out.Reset()
				if err := run(); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "calls/s")
		})
	}
}

// TestBridgeCellsAgree pins the benchmark subject: every cell has to record
// exactly one entry per call, or the benchmarks are comparing different work.
// A cell that silently stopped calling the host would otherwise look fast.
func TestBridgeCellsAgree(t *testing.T) {
	names := []string{
		"analytics_record",
		"analytics_record_fast",
		`Analytics\record`,
		"analytics_record_ctx",
	}
	for _, flat := range []bool{false, true} {
		for _, name := range names {
			t.Run(fmt.Sprintf("flat=%v/%s", flat, name), func(t *testing.T) {
				ring := tests.NewAnalyticsRing(16)
				src := bridgeProgram("once", name)
				program := mustBridgeProgram(t, src, flat)

				var out strings.Builder
				register := func(rt *runner.Runtime) {
					for n, fn := range tests.AnalyticsFuncs(ring) {
						rt.RegisterFunc(n, fn)
					}
				}
				var err error
				if flat {
					rt := flatstack.New(&out, flatstack.Options{})
					stdlib.Register(rt, register)
					err = rt.Run(program)
				} else {
					rt := runner.New(&out, runner.Options{})
					stdlib.Register(rt, register)
					err = rt.Run(program)
				}
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				if got := ring.Count(); got != 1 {
					t.Fatalf("recorded %d entries, want 1", got)
				}
				last := ring.Last()
				if last.Route != "/index.php" || last.Status != 200 || last.Micros != 1234 {
					t.Fatalf("recorded %+v, want {/index.php 200 1234}", *last)
				}
			})
		}
	}
}

// TestAnalyticsRingOverwrites pins the ring itself: it holds the last size
// entries and keeps counting past them.
func TestAnalyticsRingOverwrites(t *testing.T) {
	ring := tests.NewAnalyticsRing(4)
	for i := range 6 {
		ring.Record("/r", 200, int64(i))
	}
	if got := ring.Count(); got != 6 {
		t.Fatalf("Count() = %d, want 6", got)
	}
	snapshot := ring.Snapshot()
	if len(snapshot) != 4 {
		t.Fatalf("Snapshot() holds %d entries, want 4", len(snapshot))
	}
	for i, entry := range snapshot {
		if want := int64(i + 2); entry.Micros != want {
			t.Fatalf("Snapshot()[%d].Micros = %d, want %d", i, entry.Micros, want)
		}
	}
	if got := ring.Last().Micros; got != 5 {
		t.Fatalf("Last().Micros = %d, want 5", got)
	}
}

// TestAnalyticsRecordDoesNotAllocate pins the subject as allocation-free, so an
// allocs/op figure in docs/php-go-calls.md is the bridge's and not the ring's.
func TestAnalyticsRecordDoesNotAllocate(t *testing.T) {
	ring := tests.NewAnalyticsRing(16)
	got := testing.AllocsPerRun(100, func() {
		ring.Record("/index.php", 200, 1234)
	})
	if got != 0 {
		t.Fatalf("Record allocates %v times per call, want 0", got)
	}
}

// exprInt reads an integer expr produced, which is int for a literal and int64
// once it has been through phpscript's transpiler.
func exprInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

// contextIsUnused keeps the context import honest: the ctx binding is reached
// through the runtime, never called directly here.
var _ = context.Background
