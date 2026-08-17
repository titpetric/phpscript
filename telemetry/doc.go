// Package telemetry binds github.com/titpetric/oida into the phpscript
// namespace: traces and spans recorded in process, with a server side rendered
// front end mounted at /debug/oida.
//
// This is the only package allowed to import oida. Everything else in
// phpscript instruments through the symbols bound here, so the provider is
// named in one place and a replacement is one package wide change rather than
// a repository wide one. The bindings are type aliases and thin wrappers, so
// a *telemetry.Span is a *oida.Span: nothing is copied or adapted at runtime.
//
// A host wires it in three calls:
//
//	module, err := telemetry.NewModule(telemetry.NewOptions())
//	if err != nil {
//		return err
//	}
//	svc.Use(module.Middleware)
//	svc.Register(module)
//
// The module is also a runner.Observer, so a Runtime handed to it reports its
// scoreboard state and its spans onto the trace of the request that is running:
//
//	rt.SetContext(r.Context())
//	rt.Observe(module)
//
// Instrumentation is nil safe. Spans started without a trace in the context,
// or in a process where telemetry is disabled, return a nil span whose methods
// do nothing, so instrumented code runs unchanged either way.
package telemetry
