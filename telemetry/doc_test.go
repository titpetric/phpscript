package telemetry

import (
	"context"
	"errors"
	"testing"
)

// TestInstrumentationUnderADisabledTracer covers the nil safety this package
// claims. A tracer that was never enabled starts no trace, so Start, StartSpan
// and Do all run against a context holding none: the span is nil, every method
// on it does nothing, and Do still calls fn and returns its error unchanged.
// A binding therefore instruments without testing whether telemetry is on.
func TestInstrumentationUnderADisabledTracer(t *testing.T) {
	tracer, err := New(NewOptions("phpscript"))
	if err != nil {
		t.Fatal(err)
	}
	if tracer.Enabled() {
		t.Fatal("a tracer built from default options records, want it off until a host enables it")
	}

	ctx, trace, err := tracer.StartTrace(context.Background(), "GET /index.php")
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("StartTrace() = %v, want ErrDisabled", err)
	}
	if trace != nil {
		t.Fatalf("StartTrace() returned trace %+v, want nil", trace)
	}

	// The location a PHP frame published is still in the context, so this is
	// the path a binding takes in a process where nobody turned recording on.
	ctx = WithSpanLine(WithSpanFilename(ctx, "public/index.php"), 12)

	if span := StartSpan(ctx, "SELECT users", KindDatabase); span != nil {
		t.Fatalf("StartSpan() = %+v, want nil without a trace", span)
	}
	nested, span := Start(ctx, "render", KindTemplate)
	if span != nil {
		t.Fatalf("Start() = %+v, want nil without a trace", span)
	}

	// Every method tolerates the nil receiver, so a binding calls them
	// unconditionally.
	span.SetAttribute("rows", 3)
	span.RecordError(errors.New("ignored"))
	span.End()

	failure := errors.New("connect: refused")
	ran := false
	got := Do(nested, "connect", func(context.Context) error {
		ran = true
		return failure
	}, KindExternal)
	if !ran {
		t.Error("Do did not call fn, want the work run whether or not it is traced")
	}
	if !errors.Is(got, failure) {
		t.Errorf("Do() = %v, want the error fn returned", got)
	}
	if got := TraceID(nested); got != "" {
		t.Errorf("TraceID() = %q, want an empty string with nothing traced", got)
	}
}
