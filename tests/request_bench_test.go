package tests_test

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/titpetric/oida"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/telemetry"
)

// BenchmarkRequestCycle measures one request's runtime turnaround on a
// reused runtime: session reset, superglobal registration, and a script
// that reads them. Request parsing (FromRequest) sits outside the loop -
// it belongs to the HTTP layer, not the runtime cycle this instruments.
func BenchmarkRequestCycle(b *testing.B) {
	prog, err := parser.Parse(`<?php echo $_GET["q"], ":", $_SERVER["REQUEST_METHOD"], ":", $_REQUEST["q"];`)
	if err != nil {
		b.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/search?q=hi&page=2", nil)
	req.Header.Set("Cookie", "sid=abc123")
	ctx := runner.FromRequest(req)

	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rt.ResetSession(io.Discard, nil)
		ctx.Register(rt)
		if err := rt.Run(prog); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRequestCycleTraced is BenchmarkRequestCycle with an oida recorder
// attached and a script shaped like a real controller: one constructor and a
// method call loop, about thirty spans per request. The delta against the
// untraced benchmark is the observer's whole cost - span construction,
// recording, and the per-call name builds the observer guard skips otherwise.
func BenchmarkRequestCycleTraced(b *testing.B) {
	prog, err := parser.Parse(`<?php
class Svc {
	function step($i) {
		return $i + 1;
	}
}
$svc = new Svc;
$n = 0;
for ($i = 0; $i < 28; $i++) {
	$n = $svc->step($n);
}
echo $n, ":", $_GET["q"];`)
	if err != nil {
		b.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/search?q=hi&page=2", nil)
	reqCtx := runner.FromRequest(req)

	opts := oida.NewOptions("bench")
	opts.Enabled = true
	tracer, err := oida.New(opts)
	if err != nil {
		b.Fatal(err)
	}
	module := telemetry.NewModule(tracer)

	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)
	rt.Observe(module)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rt.ResetSession(io.Discard, nil)
		reqCtx.Register(rt)
		err := module.TrackLifecycle(context.Background(), "bench", "bench.php", func(ctx context.Context) error {
			rt.SetContext(ctx)
			return rt.Run(prog)
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRequestCycleSvc is the traced benchmark's script with no observer
// attached: the control that isolates what recording ~30 spans costs.
func BenchmarkRequestCycleSvc(b *testing.B) {
	prog, err := parser.Parse(`<?php
class Svc {
	function step($i) {
		return $i + 1;
	}
}
$svc = new Svc;
$n = 0;
for ($i = 0; $i < 28; $i++) {
	$n = $svc->step($n);
}
echo $n, ":", $_GET["q"];`)
	if err != nil {
		b.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/search?q=hi&page=2", nil)
	reqCtx := runner.FromRequest(req)

	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rt.ResetSession(io.Discard, nil)
		reqCtx.Register(rt)
		if err := rt.Run(prog); err != nil {
			b.Fatal(err)
		}
	}
}
