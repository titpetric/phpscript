package runner

import (
	"context"

	"github.com/titpetric/phpscript/telemetry"
)

// This file is the runtime's observer surface: the contract a host implements
// to watch a run, and the publications the runtime makes through it - phase
// changes, trace spans, the entrypoint and the include count.

// Observer receives lifecycle updates for a Runtime. Implementations must be
// safe for use by concurrent runtimes. Trace returns a span so the interpreter
// can measure the region it just reported; a nil span is valid and every
// method on it does nothing and an observer with no trace in the context can
// return one.
type Observer interface {
	UpdateStatus(context.Context, telemetry.State)
	Trace(context.Context, string, ...telemetry.Kind) *telemetry.Span
}

// FilenameObserver optionally receives the entrypoint passed to LoadFile.
type FilenameObserver interface {
	UpdateFilename(context.Context, string)
}

// IncludeObserver optionally receives the number of files included so far.
type IncludeObserver interface {
	UpdateIncludedFiles(context.Context, int)
}

// Observe registers an observer and reports that this Runtime is starting.
func (rt *Runtime) Observe(observer Observer) {
	if observer == nil {
		return
	}
	rt.observers = append(rt.observers, observer)
	if rt.status == "" {
		rt.status = telemetry.StateStarting
	}
	observer.UpdateStatus(rt.ctx, rt.status)
}

// UpdateStatus publishes a lifecycle phase to all registered observers. Hosts
// call it too, for phases that occur outside PHP execution.
func (rt *Runtime) UpdateStatus(state telemetry.State) {
	if rt.status == state {
		return
	}
	rt.status = state
	for _, observer := range rt.observers {
		observer.UpdateStatus(rt.ctx, state)
	}
}

// Trace publishes a span to the observers, returning the first mutable one.
func (rt *Runtime) Trace(message string, kind ...telemetry.Kind) *telemetry.Span {
	return rt.traceContext(rt.ctx, message, kind...)
}

func (rt *Runtime) traceContext(ctx context.Context, message string, kind ...telemetry.Kind) *telemetry.Span {
	var span *telemetry.Span
	for _, observer := range rt.observers {
		observed := observer.Trace(ctx, message, kind...)
		if span == nil {
			span = observed
		}
	}
	if span != nil {
		span.SetAttribute("memory_usage", rt.lastMemoryUsage())
	}
	return span
}

// UpdateFilename publishes the PHP entrypoint to observers that support
// filename updates.
func (rt *Runtime) UpdateFilename(filename string) {
	if rt.entrypoint != "" {
		return
	}
	// rootPath writes the entrypoint from the source filesystem's root, which
	// is how a script reads every path: __DIR__ . "/x" names one file wherever
	// chdir has moved the working directory, and a span, a coverage key and a
	// redeclaration message all spell the same file the same way.
	rt.entrypoint = rootPath(filename)
	for _, observer := range rt.observers {
		if filenameObserver, ok := observer.(FilenameObserver); ok {
			filenameObserver.UpdateFilename(rt.ctx, filename)
		}
	}
}

// UpdateIncludedFiles publishes the current number of included files.
func (rt *Runtime) UpdateIncludedFiles(count int) {
	for _, observer := range rt.observers {
		if includeObserver, ok := observer.(IncludeObserver); ok {
			includeObserver.UpdateIncludedFiles(rt.ctx, count)
		}
	}
}
