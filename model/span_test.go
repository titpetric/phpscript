package model

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSpanReturnsStableMutableValue(t *testing.T) {
	request := &Request{}
	ctx := WithSpanLine(WithSpanFilename(WithRequest(context.Background(), request), "views/page.tpl"), 17)
	span := StartSpan(ctx, "measured")
	if span == nil {
		t.Fatal("Span returned nil")
	}
	for i := 0; i < 100; i++ {
		StartSpan(ctx, "nested")
	}
	span.SetDuration(5 * time.Millisecond)
	if request.Spans[0] != span || request.Spans[0].Duration != 5*time.Millisecond {
		t.Fatalf("stored span was not mutable: %+v", request.Spans[0])
	}
	if span.Filename != "views/page.tpl" {
		t.Fatalf("span filename = %q", span.Filename)
	}
	if span.Line != 17 {
		t.Fatalf("span line = %d", span.Line)
	}
}

func TestSpanWithoutRequestReturnsNil(t *testing.T) {
	if span := StartSpan(context.Background(), "ignored"); span != nil {
		t.Fatalf("Span returned %+v without a request", span)
	}
}

func TestRequestTracerAndSpanMethods(t *testing.T) {
	request := &Request{}
	ctx := WithSpanFilename(context.Background(), "source.php")
	span, ok := request.StartSpan(ctx, "work").(*RequestSpan)
	if !ok {
		t.Fatalf("span = %#v", span)
	}
	started := time.Now().Add(-time.Second)
	span.SetMessage("updated")
	span.SetFilename("updated.php")
	span.SetLine(42)
	span.SetTime(started)
	span.SetType(SpanType.External)
	span.SetAttribute("attempt", int64(2))
	span.RecordError(errors.New("failed"))
	span.End()

	if span.Message != "updated" || span.Filename != "updated.php" || span.Line != 42 || !span.Time.Equal(started) || span.Duration < time.Second || span.Type != SpanType.External || span.Attributes["attempt"] != int64(2) || span.Error != "failed" {
		t.Fatalf("span = %+v", span)
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
