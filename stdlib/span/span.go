// Package span exposes telemetry spans to PHP as the start_span() binding, so
// a script can measure a region of its own work next to the spans the
// interpreter records for it.
//
// The span is the telemetry span, not a copy of it: methods on the returned
// value are the Go methods spelled the way PHP spells them.
//
//	$span = start_span("getUser", "database");
//	$span->set_attribute("user_id", 42);
//	$span->end();
//
// Without a trace in the context, which is what a CLI run without telemetry
// looks like, start_span returns a span whose methods do nothing.
package span

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

// init contributes the span bindings (start_span) to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs symbols into the runtime.
func Register(rt *runner.Runtime) {
	// start_span opens a telemetry span named $name, of kind $kind when given,
	// and returns it; the script closes it with $span->end(). Without a trace
	// in the context, the returned span's methods do nothing.
	rt.RegisterFunc("start_span", telemetry.StartSpan)
}
