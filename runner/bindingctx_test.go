package runner

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/titpetric/phpscript/telemetry"
)

type middlewareKey struct{}

// TestBindingContextDerivesFromTheRequest covers the chain a binding's context
// resolves through while a request is live: the frame and the environment from
// the value's own fields, the active span from the session chain, and a
// middleware value, with the cancellation, from the request. A span started
// from the binding context lands on the session's trace and nests below the
// active region, so a database call inside a traced request stays under the
// request's own span.
func TestBindingContextDerivesFromTheRequest(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})

	traceOptions := telemetry.NewOptions("phpscript")
	traceOptions.Enabled = true
	tracer, err := telemetry.New(traceOptions)
	if err != nil {
		t.Fatal(err)
	}
	sessionCtx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}
	rt.SetContext(sessionCtx)

	// What rt.trace does around a call region: the span goes onto the session
	// chain and comes off it when the region ends.
	region, regionSpan := telemetry.Start(rt.Context(), "handler")
	restore := rt.ctx
	rt.ctx = region
	defer func() { rt.ctx = restore }()

	requestCtx, disconnect := context.WithCancel(
		context.WithValue(context.Background(), middlewareKey{}, "from middleware"))
	defer disconnect()
	leave := rt.EnterRequest(requestCtx)

	scope := NewScope()
	ctx := rt.bindingContext(scope)

	if got, ok := ScopeFromContext(ctx); !ok || got != scope {
		t.Errorf("ScopeFromContext() = %v, %v, want the calling frame", got, ok)
	}
	if got := ctx.Value(middlewareKey{}); got != "from middleware" {
		t.Errorf("middleware value = %v, want it read through the request chain", got)
	}
	if got := telemetry.SpanFromContext(ctx); got != regionSpan {
		t.Errorf("SpanFromContext() = %v, want the active region's span", got)
	}

	nested := telemetry.StartSpan(ctx, "db.query", telemetry.KindDatabase)
	if nested == nil {
		t.Fatal("a span started from the binding context recorded nothing")
	}
	nested.End()
	regionSpan.End()
	spans := trace.Clone().Spans
	if len(spans) < 3 {
		t.Fatalf("spans = %+v, want the lifecycle, the region and the query", spans)
	}
	query := spans[len(spans)-1]
	if query.Name != "db.query" || query.ParentID != regionSpan.ID {
		t.Errorf("query span = %+v, want it nested below span %d", query, regionSpan.ID)
	}

	disconnect()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("the disconnect does not reach the binding context")
	}

	leave()
	if got := rt.bindingContext(scope).Value(middlewareKey{}); got != nil {
		t.Errorf("after the request, middleware value = %v, want none", got)
	}
}

// TestBindingContextIgnoreUserAbortDetaches covers the handler that asked to
// finish: the binding context keeps the request's values and drops its
// cancellation, so a disconnect ends no binding work.
func TestBindingContextIgnoreUserAbortDetaches(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})

	requestCtx, disconnect := context.WithCancel(
		context.WithValue(context.Background(), middlewareKey{}, "kept"))
	leave := rt.EnterRequest(requestCtx)
	defer leave()
	rt.SetIgnoreUserAbort(true)

	ctx := rt.bindingContext(NewScope())
	disconnect()

	if ctx.Done() != nil {
		t.Error("Done() answers a channel; a detached context has none")
	}
	if err := ctx.Err(); err != nil {
		t.Errorf("Err() = %v, want nil on a detached context", err)
	}
	if _, ok := ctx.Deadline(); ok {
		t.Error("Deadline() reports one a detached context does not carry")
	}
	if got := ctx.Value(middlewareKey{}); got != "kept" {
		t.Errorf("middleware value = %v, want it kept through the detach", got)
	}
}
