package ps

import (
	"context"
	"testing"

	"github.com/titpetric/phpscript/telemetry"
)

func TestSessionStorageRecordsSpans(t *testing.T) {
	tracer, err := telemetry.New(telemetry.NewOptions())
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}

	manager, err := NewSessionManager(NewSessionStorageMemory())
	if err != nil {
		t.Fatal(err)
	}
	storage := manager.storage

	const id = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := storage.Save(ctx, id, []byte("42")); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Load(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Load(ctx, "missing"); err == nil {
		t.Fatal("loading an unknown session returned no error")
	}

	spans := trace.Clone().Spans[1:]
	if len(spans) != 3 {
		t.Fatalf("spans = %+v", spans)
	}
	for _, span := range spans {
		if span.Kind != telemetry.KindCache || !span.Ended() {
			t.Fatalf("session span = %+v", span)
		}
	}
	if spans[0].Name != "session save" || spans[0].Attributes["bytes"] != 2 {
		t.Fatalf("save span = %+v", spans[0])
	}
	if spans[1].Name != "session load" || spans[1].Attributes["hit"] != true || spans[1].Attributes["bytes"] != 2 {
		t.Fatalf("load span = %+v", spans[1])
	}

	// A session that is not there is a miss, not a failure: it must not fail
	// the trace, and the session ID must never reach the front end.
	if spans[2].Attributes["hit"] != false || spans[2].Failed() {
		t.Fatalf("miss span = %+v", spans[2])
	}
	for _, span := range spans {
		for key, value := range span.Attributes {
			if value == id {
				t.Fatalf("span %q recorded the session ID in %q", span.Name, key)
			}
		}
	}
}
