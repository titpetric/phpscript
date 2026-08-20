package telemetry

import (
	"context"
)

// Module observes the PHP interpreter and records what it reports onto the
// trace of the request that is running: the scoreboard state, the entrypoint
// it resolved to, and one span per include, call or template.
//
// It is not a recorder. The host platform registers one, and the tracing
// middleware that recorder installs is what puts a trace in the request
// context; this type only writes onto it. That is why interpreter work shows
// up on the platform's debug front end without phpscript mounting one.
type Module struct {
	tracer *Tracer
}

// NewModule returns an observer recording into tracer, which is the tracer the
// host recorder built. A nil tracer is valid and means nothing is recorded:
// every call below is nil safe, so instrumented code runs unchanged either way.
func NewModule(tracer *Tracer) *Module {
	return &Module{tracer: tracer}
}

// Tracer returns the tracer the module records into.
func (m *Module) Tracer() *Tracer {
	return m.tracer
}

// Snapshot returns a race free copy of everything recorded so far.
func (m *Module) Snapshot() Snapshot {
	return m.tracer.Snapshot()
}

// TrackLifecycle records work that did not arrive over the network, such as a
// @startup file, as a trace of its own. There is no request to record onto, so
// this is the one place the observer starts a trace rather than writing to one.
func (m *Module) TrackLifecycle(ctx context.Context, name, filename string, run func(context.Context) error) error {
	return m.tracer.Observe(ctx, name, func(ctx context.Context) error {
		trace := TraceFromContext(ctx)
		trace.Root().SetSource(filename, 0)

		err := run(WithSpanFilename(ctx, filename))
		if err != nil {
			trace.Root().RecordError(err)
			trace.SetState(StateError)
		}
		return err
	})
}

// UpdateStatus implements runner.Observer: it moves the trace of the running
// request to the scoreboard state the interpreter reports.
func (m *Module) UpdateStatus(ctx context.Context, state State) {
	TraceFromContext(ctx).SetState(state)
}

// Trace implements runner.Observer: it records one span of interpreter work,
// such as an include, a call or a template, on the running trace.
func (m *Module) Trace(ctx context.Context, message string, kind ...Kind) *Span {
	return StartSpan(ctx, message, kind...)
}

// UpdateFilename records the PHP entrypoint of the running request. Included
// files do not replace it: the entrypoint is the file the request resolved to.
func (m *Module) UpdateFilename(ctx context.Context, filename string) {
	if filename == "" {
		return
	}
	root := TraceFromContext(ctx).Root()
	root.SetAttribute("filename", filename)
	if root.SourceText() == "" {
		root.SetSource(filename, 0)
	}
}

// UpdateIncludedFiles records how many files the request included beyond its
// entrypoint.
func (m *Module) UpdateIncludedFiles(ctx context.Context, count int) {
	TraceFromContext(ctx).Root().SetAttribute("included_files", count)
}
