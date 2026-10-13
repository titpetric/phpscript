package telemetry

import (
	"context"
	"errors"
	"testing"
)

// newRecordingTracer returns a tracer that records. oida records only when it
// is enabled, and a test that reads its traces back has to enable it.
func newRecordingTracer(t *testing.T) *Tracer {
	t.Helper()

	options := NewOptions("phpscript")
	options.Enabled = true
	tracer, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	return tracer
}

// TestTrace covers what one recorded unit of work holds. The alias is for
// oida.Trace, so a span recorded through this package lands on the value the
// debug front end renders: a root span opened under the name of the trace,
// with one span per instrumented call below it.
func TestTrace(t *testing.T) {
	tracer := newRecordingTracer(t)
	ctx, trace, err := tracer.StartTrace(context.Background(), "GET /index.php")
	if err != nil {
		t.Fatal(err)
	}

	StartSpan(ctx, "SELECT users", KindDatabase).End()
	tracer.Finish(trace)

	recorded := trace.Clone()
	if recorded.Name != "GET /index.php" {
		t.Errorf("Name = %q, want the name the trace was started under", recorded.Name)
	}
	if recorded.ID == "" {
		t.Error("ID is empty, so no log line can be matched with this trace")
	}
	if len(recorded.Spans) != 2 {
		t.Fatalf("spans = %+v, want the root span and the one below it", recorded.Spans)
	}
	if recorded.Spans[0].Name != "GET /index.php" || recorded.Spans[0].Kind != KindInternal {
		t.Errorf("root span = %+v", recorded.Spans[0])
	}
	if recorded.Spans[1].Name != "SELECT users" || recorded.Spans[1].Kind != KindDatabase {
		t.Errorf("recorded span = %+v", recorded.Spans[1])
	}
	if !recorded.Spans[1].Ended() || recorded.Spans[1].Duration <= 0 {
		t.Errorf("recorded span was not measured: %+v", recorded.Spans[1])
	}
}

// TestDo covers a call wrapped in a span. Do records what fn returned on the
// span and ends it, and returns the error unchanged, so the error handling of
// an instrumented caller reads the same as before it was instrumented. The
// context fn runs under carries the span, so work below it nests.
func TestDo(t *testing.T) {
	tracer := newRecordingTracer(t)
	ctx, trace, err := tracer.StartTrace(context.Background(), "job")
	if err != nil {
		t.Fatal(err)
	}

	var inner context.Context
	if err := Do(ctx, "warm cache", func(ctx context.Context) error {
		inner = ctx
		return nil
	}, KindCache); err != nil {
		t.Fatalf("Do() = %v on a call that succeeded", err)
	}
	if span := SpanFromContext(inner); span == nil || span.Name != "warm cache" {
		t.Errorf("fn ran under span %+v, want the one Do opened", span)
	}

	failure := errors.New("connect: refused")
	if got := Do(ctx, "connect", func(context.Context) error {
		return failure
	}, KindExternal); !errors.Is(got, failure) {
		t.Fatalf("Do() = %v, want the error fn returned", got)
	}

	spans := trace.Clone().Spans
	if len(spans) != 3 {
		t.Fatalf("spans = %+v, want the root span and one per call", spans)
	}
	if spans[1].Name != "warm cache" || spans[1].Kind != KindCache || spans[1].Err() != nil {
		t.Errorf("span of the call that succeeded = %+v", spans[1])
	}
	if spans[2].Name != "connect" || spans[2].Kind != KindExternal {
		t.Fatalf("span of the call that failed = %+v", spans[2])
	}
	if !spans[2].Ended() || spans[2].ErrorText != failure.Error() {
		t.Errorf("span = %+v, want it ended with the error recorded on it", spans[2])
	}
}

// TestTraceFromContext covers reading a trace back out of a context. A context
// with no trace in it answers nil, and every method on a nil trace does
// nothing, so instrumented code runs unchanged where telemetry is off. A
// context the tracer did not derive carries one once WithTrace put it there,
// the spelling a host uses to pass an existing trace to the interpreter.
func TestTraceFromContext(t *testing.T) {
	if got := TraceFromContext(context.Background()); got != nil {
		t.Errorf("TraceFromContext() = %+v on a bare context, want nil", got)
	}

	tracer := newRecordingTracer(t)
	ctx, trace, err := tracer.StartTrace(context.Background(), "GET /index.php")
	if err != nil {
		t.Fatal(err)
	}
	if got := TraceFromContext(ctx); got != trace {
		t.Errorf("TraceFromContext() = %p, want the trace %p", got, trace)
	}

	carried := WithTrace(context.Background(), trace)
	if got := TraceFromContext(carried); got != trace {
		t.Errorf("TraceFromContext() = %p after WithTrace, want %p", got, trace)
	}
}

// TestSpanFromContext covers which span a nested call records under: the
// innermost one in the context. Start derives a context carrying the span it
// opened, so work below it nests; StartSpan derives none, so a leaf span
// leaves the context naming its parent and nothing nests under a span that
// has already been ended.
func TestSpanFromContext(t *testing.T) {
	if got := SpanFromContext(context.Background()); got != nil {
		t.Errorf("SpanFromContext() = %+v on a bare context, want nil", got)
	}

	tracer := newRecordingTracer(t)
	ctx, _, err := tracer.StartTrace(context.Background(), "GET /index.php")
	if err != nil {
		t.Fatal(err)
	}

	root := SpanFromContext(ctx)
	if root == nil || root.Name != "GET /index.php" {
		t.Fatalf("SpanFromContext() = %+v, want the root span", root)
	}

	nested, span := Start(ctx, "render", KindTemplate)
	if got := SpanFromContext(nested); got != span {
		t.Errorf("SpanFromContext() = %p, want the span Start opened, %p", got, span)
	}
	span.End()

	leaf := StartSpan(ctx, "SELECT users", KindDatabase)
	if got := SpanFromContext(ctx); got != root {
		t.Errorf("SpanFromContext() = %p after StartSpan, want the parent %p", got, root)
	}
	leaf.End()
}

// TestTraceID covers the correlation key a log line carries. It is the value of
// the Request-Id header on an HTTP trace, so one string matches a log line with
// the trace it was written under, and it is empty where nothing is traced.
func TestTraceID(t *testing.T) {
	if got := TraceID(context.Background()); got != "" {
		t.Errorf("TraceID() = %q on a bare context, want an empty string", got)
	}

	tracer := newRecordingTracer(t)
	ctx, trace, err := tracer.StartTrace(context.Background(), "GET /index.php")
	if err != nil {
		t.Fatal(err)
	}
	if got := TraceID(ctx); got != trace.ID {
		t.Errorf("TraceID() = %q, want the trace identifier %q", got, trace.ID)
	}
}

// TestNewStorageDisk covers the retention a site that configured `driver: disk`
// is held to: completed traces written as JSON documents under the named
// folder, bounded by the limit, and readable by a process that did not record
// them. Restore is what fills the read path of a fresh process, so a restart
// keeps what the last one recorded.
func TestNewStorageDisk(t *testing.T) {
	root := t.TempDir()
	store, err := NewStorageDisk(2, root)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Cap(); got != 2 {
		t.Errorf("Cap() = %d, want the limit the storage was built with", got)
	}

	tracer := newRecordingTracer(t)
	_, trace, err := tracer.StartTrace(context.Background(), "GET /index.php")
	if err != nil {
		t.Fatal(err)
	}
	tracer.Finish(trace)

	if err := store.Save(t.Context(), trace); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Len(t.Context()); err != nil || got != 1 {
		t.Fatalf("Len() = %d, %v, want 1 retained trace", got, err)
	}

	reopened, err := NewStorageDisk(2, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Restore(t.Context()); err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load(t.Context(), trace.ID)
	if err != nil {
		t.Fatalf("Load() = %v, want the trace the earlier process wrote", err)
	}
	if loaded.ID != trace.ID || loaded.Name != "GET /index.php" {
		t.Errorf("Load() = %+v, want the trace that was saved", loaded)
	}
}
