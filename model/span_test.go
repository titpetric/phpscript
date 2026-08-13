package model

import (
	"context"
	"testing"
	"time"
)

func TestSpanReturnsStableMutableValue(t *testing.T) {
	request := &Request{}
	ctx := WithSpanFilename(WithRequest(context.Background(), request), "views/page.tpl")
	span := Span(ctx, "measured")
	if span == nil {
		t.Fatal("Span returned nil")
	}
	for i := 0; i < 100; i++ {
		Span(ctx, "nested")
	}
	span.Duration = 5 * time.Millisecond
	if request.Spans[0] != span || request.Spans[0].Duration != 5*time.Millisecond {
		t.Fatalf("stored span was not mutable: %+v", request.Spans[0])
	}
	if span.Filename != "views/page.tpl" {
		t.Fatalf("span filename = %q", span.Filename)
	}
}

func TestSpanWithoutRequestReturnsNil(t *testing.T) {
	if span := Span(context.Background(), "ignored"); span != nil {
		t.Fatalf("Span returned %+v without a request", span)
	}
}

func TestClosingSpanMeasuresNestedOpenSpan(t *testing.T) {
	started := time.Unix(1_700_000_000, 0)
	request := &Request{}
	outer := request.AppendSpan(started, "outer", OpenSpan)
	inner := request.AppendSpan(started.Add(time.Millisecond), "inner", OpenSpan)
	innerClose := request.AppendSpan(started.Add(3*time.Millisecond), "inner", CloseSpan)
	outerClose := request.AppendSpan(started.Add(5*time.Millisecond), "outer", CloseSpan)

	if inner.Duration != 2*time.Millisecond || outer.Duration != 5*time.Millisecond {
		t.Fatalf("region durations: outer=%s inner=%s", outer.Duration, inner.Duration)
	}
	if innerClose.Duration != 0 || outerClose.Duration != 0 {
		t.Fatalf("closing spans have durations: outer=%s inner=%s", outerClose.Duration, innerClose.Duration)
	}
}
