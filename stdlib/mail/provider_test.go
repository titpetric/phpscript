package mail_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	mailstdlib "github.com/titpetric/phpscript/stdlib/mail"
	"github.com/titpetric/phpscript/telemetry"
)

// Mail leaves the process, so it is recorded as external work on the trace of
// the request that sent it, whether the script used mail() or `new Mail`.
func TestMailRecordsAnExternalSpan(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		script string
		bytes  int
	}{
		{
			name:   "mail",
			script: `<?php mail("recipient@example.com", "Subject", "Body line");`,
			bytes:  len("Body line"),
		},
		{
			name: "Mail",
			script: `<?php
				$mail = new Mail("marketing");
				$mail->send("recipient@example.com", "Subject", "Body line");
			`,
			bytes: len("Body line"),
		},
		{
			name: "Mail with HTML",
			script: `<?php
				$mail = new Mail("marketing");
				$mail->send("recipient@example.com", "Subject", "Body line", "<p>Body line</p>");
			`,
			bytes: len("Body line") + len("<p>Body line</p>"),
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

			queue := mailstdlib.NewMemory("default", "marketing")
			rt := runner.New(&bytes.Buffer{}, runner.Options{Mail: queue})
			rt.SetContext(ctx)
			stdlib.Register(rt)
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
			if span.Attributes["bytes"] != test.bytes {
				t.Fatalf("recorded body size = %#v, want %d", span.Attributes["bytes"], test.bytes)
			}
			for key, value := range span.Attributes {
				if value == "Body line" || value == "<p>Body line</p>" {
					t.Fatalf("the message body was recorded in %q", key)
				}
			}
		})
	}
}

// TestProvider covers what a host builds from its mail block: a fixed set of
// named servers, keyed by the name a script types. Names are lowercased and a
// missing port is filled in while the provider is built, so a delivery reads a
// complete credential. The map is copied, because configuration layering hands
// one map to every site that declared no mail block, and a normalisation
// written into it would be written into all of them.
func TestProvider(t *testing.T) {
	t.Parallel()
	servers := map[string]mailstdlib.Config{
		"Marketing": {Host: "smtp.example.com", From: "news@example.com"},
	}
	provider := mailstdlib.NewProvider(servers)

	if err := provider.Configured("marketing"); err != nil {
		t.Errorf("Configured(marketing) = %v, want nil for the server that was named", err)
	}
	if _, held := servers["marketing"]; held {
		t.Error("the provider wrote its lowercased name back into the caller's map")
	}
	if got := servers["Marketing"].Port; got != 0 {
		t.Errorf("the provider wrote port %d back into the caller's map", got)
	}

	// A provider holding nothing is what a CLI run without a mail block has.
	if err := mailstdlib.NewProvider(nil).Configured(""); err == nil {
		t.Error("an empty provider accepted the default server")
	}
}

// TestProvider_Configured covers the verdict the constructor reads, so `new
// Mail("marketing")` fails where the delivery would. An empty name is the
// "default" server, as `new Mail` and mail() both resolve. A name nobody
// configured is reported by the name the call passes, and the error says
// nothing about the configuration.
func TestProvider_Configured(t *testing.T) {
	t.Parallel()
	provider := mailstdlib.NewProvider(map[string]mailstdlib.Config{
		"default":   {Host: "smtp.example.com", From: "root@example.com"},
		"marketing": {Host: "smtp.example.com", From: "news@example.com"},
	})

	for _, name := range []string{"", "default", "marketing", "MARKETING"} {
		if err := provider.Configured(name); err != nil {
			t.Errorf("Configured(%q) = %v, want nil", name, err)
		}
	}

	err := provider.Configured("billing")
	if err == nil {
		t.Fatal("Configured(billing) = nil for a server nobody configured")
	}
	if want := "no configuration found for mail server: billing"; err.Error() != want {
		t.Errorf("Configured(billing) = %q, want %q", err, want)
	}
	if strings.Contains(err.Error(), "smtp.example.com") {
		t.Errorf("the refusal names a configured host: %v", err)
	}
}

// TestProvider_Send covers the delivery path: a name resolves to a credential,
// the credential reaches the transport, and the credential is gone when the
// call returns. A name nobody configured never reaches the transport, so the
// refusal costs no connection.
func TestProvider_Send(t *testing.T) {
	t.Parallel()
	var sent []string
	provider := mailstdlib.NewProviderFunc(func(config mailstdlib.Config, recipient, subject, body string) error {
		sent = append(sent, config.Host+" "+recipient+" "+subject+" "+body)
		return nil
	}, map[string]mailstdlib.Config{
		"default": {Host: "smtp.example.com", From: "root@example.com", Password: "secret"},
	})

	if err := provider.Send(context.Background(), "", "a@example.com", "Hello", "Body"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	want := "smtp.example.com a@example.com Hello Body"
	if len(sent) != 1 || sent[0] != want {
		t.Fatalf("the transport read %q, want %q", sent, want)
	}

	if err := provider.Send(context.Background(), "billing", "a@example.com", "Hello", "Body"); err == nil {
		t.Error("Send through an unconfigured name = nil, want the refusal")
	}
	if len(sent) != 1 {
		t.Errorf("the transport ran %d times, want 1", len(sent))
	}
}

// TestProvider_SendMessage covers a whole message against a replaced
// transport. A Deliver reads the three parts of a plain-text message, so a
// message with an HTML part or a Reply-To is refused before the transport runs;
// cutting it down to the text would deliver something the script did not send.
func TestProvider_SendMessage(t *testing.T) {
	t.Parallel()
	var sent []string
	provider := mailstdlib.NewProviderFunc(func(_ mailstdlib.Config, recipient, subject, body string) error {
		sent = append(sent, recipient+" "+subject+" "+body)
		return nil
	}, map[string]mailstdlib.Config{
		"default": {Host: "smtp.example.com", From: "root@example.com"},
	})
	ctx := context.Background()

	if err := provider.SendMessage(ctx, mailstdlib.Message{Recipient: "a@example.com", Subject: "Hello", Body: "Body"}); err != nil {
		t.Fatalf("SendMessage, plain: %v", err)
	}
	for _, message := range []mailstdlib.Message{
		{Recipient: "a@example.com", Subject: "Hello", Body: "Body", HTML: "<p>Body</p>"},
		{Recipient: "a@example.com", Subject: "Hello", Body: "Body", ReplyTo: "lead@example.com"},
	} {
		err := provider.SendMessage(ctx, message)
		if err == nil || !strings.Contains(err.Error(), "plain text only") {
			t.Errorf("SendMessage(%+v) = %v, want the plain-text refusal", message, err)
		}
	}
	if want := "a@example.com Hello Body"; len(sent) != 1 || sent[0] != want {
		t.Fatalf("the transport read %q, want %q alone", sent, want)
	}

	err := provider.SendMessage(ctx, mailstdlib.Message{Server: "billing", Recipient: "a@example.com"})
	if err == nil || !strings.Contains(err.Error(), "no configuration found for mail server: billing") {
		t.Errorf("SendMessage through an unconfigured name = %v, want the refusal", err)
	}
}

// TestDeliver covers the one function a host replaces to send by a transport
// that is not SMTP. It reads the credential the name resolved to, with the
// port already filled in, and the three parts of the message; name resolution
// and tenant isolation sit above it, so a transport does none of its own.
func TestDeliver(t *testing.T) {
	t.Parallel()
	var (
		got                      mailstdlib.Config
		recipient, subject, body string
	)
	var deliver mailstdlib.Deliver = func(config mailstdlib.Config, to, title, text string) error {
		got, recipient, subject, body = config, to, title, text
		return nil
	}

	provider := mailstdlib.NewProviderFunc(deliver, map[string]mailstdlib.Config{
		"marketing": {Host: "smtp.example.com", From: "news@example.com"},
	})
	if err := provider.Send(context.Background(), "marketing", "list@example.com", "Newsletter", "Issue 1"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got.Host != "smtp.example.com" || got.Port != mailstdlib.DefaultPort {
		t.Errorf("the transport read %+v, want the default port filled in", got)
	}
	if recipient != "list@example.com" || subject != "Newsletter" || body != "Issue 1" {
		t.Errorf("the transport read %q, %q, %q", recipient, subject, body)
	}
}

// TestNewProviderFunc covers the transport swap, the way
// database.NewDatabaseProvider takes its connector. A host passing nil sends
// over SMTP, so the zero value of the option is the behaviour of a host that
// said nothing about transport: the dial refused by a port nobody listens on
// carries the SMTP path's own wording.
func TestNewProviderFunc(t *testing.T) {
	t.Parallel()
	provider := mailstdlib.NewProviderFunc(nil, map[string]mailstdlib.Config{
		"default": {Host: "127.0.0.1", Port: 1, From: "root@example.com"},
	})

	if err := provider.Configured("default"); err != nil {
		t.Fatalf("Configured(default) = %v", err)
	}

	err := provider.Send(context.Background(), "default", "a@example.com", "Hello", "Body")
	if err == nil {
		t.Fatal("Send over SMTP to a closed port = nil")
	}
	if !strings.Contains(err.Error(), "failed to send email") {
		t.Errorf("Send = %v, want the SMTP path's wording", err)
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
