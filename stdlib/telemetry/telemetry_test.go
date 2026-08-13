package telemetry_test

import (
	"testing"
	"time"

	"github.com/titpetric/phpscript/stdlib/telemetry"
)

func TestTelemetryRingBuffer(t *testing.T) {
	rb := telemetry.NewRingBuffer(5)

	if len(rb.Spans()) != 0 {
		t.Fatalf("expected empty buffer, got %d", len(rb.Spans()))
	}

	for i := 0; i < 10; i++ {
		s := rb.AcquireSpan("ulid-1", "test-span", "unit-test")
		time.Sleep(1 * time.Millisecond)
		rb.Finish(s)
	}

	spans := rb.Spans()
	if len(spans) != 5 {
		t.Fatalf("expected capacity bound of 5 spans, got %d", len(spans))
	}

	for _, s := range spans {
		if s.DurationNs <= 0 {
			t.Fatalf("expected positive duration, got %d", s.DurationNs)
		}
	}

	rb.Clear()
	if len(rb.Spans()) != 0 {
		t.Fatalf("expected 0 spans after Clear(), got %d", len(rb.Spans()))
	}
}
