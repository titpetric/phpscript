package telemetry

import (
	"context"
)

// Module observes the PHP interpreter and records its reports onto the trace
// of the running request: the scoreboard state, the entrypoint it resolved to,
// plus one span per include, call or template.
//
// This is no recorder. The host platform registers a recorder; the tracing
// middleware it installs puts a trace in the request context. This type writes
// onto that trace only. So interpreter work shows up on the
// platform's debug front end without phpscript mounting one.
type Module struct {
	tracer *Tracer
}

// NewModule returns an observer recording into tracer, nil recording nothing.
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

// TrackLifecycle records work that stays off the network, such as a @startup
// file, as a separate trace. No request exists to record onto. This is the
// only place the observer starts a trace. It writes no span to that trace.
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

// UpdateStatus moves the running request's trace to the reported state.
func (m *Module) UpdateStatus(ctx context.Context, state State) {
	TraceFromContext(ctx).SetState(state)
}

// Trace records one span of interpreter work on the running trace.
func (m *Module) Trace(ctx context.Context, message string, kind ...Kind) *Span {
	return StartSpan(ctx, message, kind...)
}

// UpdateFilename records the PHP entrypoint of the running request. Included
// files do not replace that: the entrypoint is the file the request resolved to.
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

// UpdateIncludedFiles records the files included past the entrypoint.
func (m *Module) UpdateIncludedFiles(ctx context.Context, count int) {
	TraceFromContext(ctx).Root().SetAttribute("included_files", count)
}
