package logger

import (
	"context"
	"errors"
	"testing"

	"github.com/titpetric/phpscript/telemetry"
)

// TestLoggerLeavesTheSpanOpen covers the half of the contract the recording
// stops at: Error fails the span in the logger's context and never ends it.
// A library is handed a logger for the length of one call while the caller
// above it still has work to report on that span, and a span ended from under
// that caller would close the region before the work it measures finished.
func TestLoggerLeavesTheSpanOpen(t *testing.T) {
	t.Parallel()
	traceOptions := telemetry.NewOptions("phpscript")
	// oida records only when it is asked to; a test that reads its
	// traces back is asking.
	traceOptions.Enabled = true
	tracer, err := telemetry.New(traceOptions)
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}

	ctx, span := telemetry.Start(ctx, "migrate", telemetry.KindDatabase)
	discard(New(ctx, "migrate")).Error("failed", "error", errors.New("syntax error"))

	// Read before the caller ends it: the span carries the failure and is
	// still open, so the duration it reports is the work and not the log line.
	recorded := trace.Clone().Spans[1]
	if recorded.Err() == nil {
		t.Error("the span did not record the error the logger reported")
	}
	if recorded.Ended() {
		t.Fatal("Error ended the span, which belongs to whoever opened it")
	}

	span.End()
	if ended := trace.Clone().Spans[1]; !ended.Ended() || ended.Duration <= 0 {
		t.Errorf("after the caller ended it, span = %+v", ended)
	}
}

// TestLoggerRecordsNothingForInfo covers the boundary between the log and the
// trace: an info line is log output, so it reaches slog alone. Recording every
// message would make the trace a second copy of the log and cost a write per
// line on a request that logged a hundred of them.
func TestLoggerRecordsNothingForInfo(t *testing.T) {
	t.Parallel()
	traceOptions := telemetry.NewOptions("phpscript")
	traceOptions.Enabled = true
	tracer, err := telemetry.New(traceOptions)
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}

	ctx, span := telemetry.Start(ctx, "migrate", telemetry.KindDatabase)
	log := discard(New(ctx, "migrate"))
	for _, file := range []string{"0001.up.sql", "0002.up.sql", "0003.up.sql"} {
		log.Info("migration", "file", file, "status", "OK")
	}
	span.End()

	// Three lines logged, one span recorded, no attributes and no error.
	spans := trace.Clone().Spans[1:]
	if len(spans) != 1 {
		t.Fatalf("spans = %+v, want the one the work opened", spans)
	}
	if spans[0].Err() != nil || len(spans[0].Attributes) != 0 {
		t.Errorf("span = %+v, want no error and no attributes", spans[0])
	}
}
