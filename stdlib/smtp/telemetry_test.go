package smtp_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	smtpstdlib "github.com/titpetric/phpscript/stdlib/smtp"
	"github.com/titpetric/phpscript/telemetry"
)

// Mail leaves the process, so it is recorded as external work on the trace of
// the request that sent it, whether the script used mail() or `new SMTP`.
func TestMailRecordsAnExternalSpan(t *testing.T) {
	for _, test := range []struct {
		name     string
		script   string
		register func(*runner.Runtime, smtpstdlib.Sender)
		wantHost string
	}{
		{
			name:     "mail",
			script:   `<?php mail("recipient@example.com", "Subject", "Body line");`,
			register: smtpstdlib.Register,
		},
		{
			name: "SMTP",
			script: `<?php
				$smtp = new SMTP(array("host" => "mail.example.com", "from" => "noreply@example.com"));
				$smtp->send("recipient@example.com", "Subject", "Body line");
			`,
			wantHost: "mail.example.com",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
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

			program, err := parser.Parse(test.script)
			if err != nil {
				t.Fatal(err)
			}

			queue := smtpstdlib.NewMemory()
			rt := runner.New(&bytes.Buffer{}, runner.Options{})
			rt.SetContext(smtpstdlib.SenderContext(ctx, queue))
			stdlib.Register(rt)
			if test.register != nil {
				test.register(rt, queue)
			}
			if err := rt.Run(program); err != nil {
				t.Fatal(err)
			}
			if messages := queue.Messages(); len(messages) != 1 {
				t.Fatalf("messages = %#v", messages)
			}

			span := lastSpan(t, trace)
			if span.Name != "mail" || span.Kind != telemetry.KindExternal || !span.Ended() {
				t.Fatalf("mail span = %+v", span)
			}
			if span.Attributes["to"] != "recipient@example.com" || span.Attributes["subject"] != "Subject" {
				t.Fatalf("mail span attributes = %+v", span.Attributes)
			}

			// The body is private; its size is the part worth recording.
			if span.Attributes["bytes"] != len("Body line") {
				t.Fatalf("recorded body size = %#v", span.Attributes["bytes"])
			}
			for key, value := range span.Attributes {
				if value == "Body line" {
					t.Fatalf("the message body was recorded in %q", key)
				}
			}
			if span.Attributes["host"] != nil && span.Attributes["host"] != test.wantHost {
				t.Fatalf("recorded host = %#v, want %q", span.Attributes["host"], test.wantHost)
			}
		})
	}
}

// lastSpan returns the span recorded last on a trace.
func lastSpan(t *testing.T, trace *telemetry.Trace) *telemetry.Span {
	t.Helper()

	spans := trace.Clone().Spans
	if len(spans) < 2 {
		t.Fatalf("spans = %+v", spans)
	}
	return spans[len(spans)-1]
}
