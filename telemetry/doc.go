// Package telemetry binds github.com/titpetric/oida into the phpscript
// namespace: traces and spans recorded in process, with a server side rendered
// front end mounted at /debug/oida.
//
// This is the only package in phpscript that imports oida. Everything else
// instruments through the symbols bound here and no call site names the
// provider. The bindings are type aliases and thin wrappers and a
// *telemetry.Span is a *oida.Span: no copy or adapter exists at runtime.
//
// That covers the call sites, and no more of the dependency. The recorder and the
// front end belong to the host platform, which names oida itself and replacing
// the provider means replacing it there as well. A host passes the tracer
// that platform built:
//
//	var recorder *platform.TelemetryModule
//	if svc.Find(&recorder) {
//		module = telemetry.NewModule(recorder.Tracer())
//	}
//
// The module is a runner.Observer and a Runtime handed to it reports its
// scoreboard state and its spans onto the trace of the request that is running:
//
//	rt.SetContext(r.Context())
//	rt.Observe(module)
//
// Instrumentation is nil safe. Spans started without a trace in the context,
// or in a process running with telemetry off, return a nil span whose methods
// do nothing and instrumented code runs unchanged either way.
package telemetry
