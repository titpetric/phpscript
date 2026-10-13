package mail_test

import (
	"context"
	"testing"

	mailstdlib "github.com/titpetric/phpscript/stdlib/mail"
)

// TestMemory_Configured covers which names an in-memory provider accepts.
// Naming none configures every name, so a dry run captures whatever a script
// sends; naming some lets a test read back the refusal a script reads for the
// rest. An empty name is the "default" server, as it is everywhere else.
func TestMemory_Configured(t *testing.T) {
	for _, test := range []struct {
		name      string
		names     []string
		server    string
		wantError bool
	}{
		{name: "catch all", server: "marketing"},
		{name: "catch all, unnamed", server: ""},
		{name: "configured", names: []string{"marketing"}, server: "marketing"},
		{name: "configured, mixed case", names: []string{"Marketing"}, server: "MARKETING"},
		{name: "unconfigured", names: []string{"marketing"}, server: "billing", wantError: true},
		{name: "unconfigured default", names: []string{"marketing"}, server: "", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := mailstdlib.NewMemory(test.names...).Configured(test.server)
			if gotError := err != nil; gotError != test.wantError {
				t.Fatalf("Configured(%q) = %v", test.server, err)
			}
		})
	}
}

// TestMemory_Send covers queueing in place of delivering. A queued message
// carries the name of the server it was addressed to, so a test reads back
// which server a script selected. A name this provider does not hold is
// refused the way a real delivery refuses it, so a test still exercises the
// error a script reads.
func TestMemory_Send(t *testing.T) {
	queue := mailstdlib.NewMemory("default", "marketing")
	ctx := context.Background()

	if err := queue.Send(ctx, "", "a@example.com", "Hello", "Body"); err != nil {
		t.Fatalf("Send through the default server: %v", err)
	}
	if err := queue.Send(ctx, "marketing", "list@example.com", "News", "Issue 1"); err != nil {
		t.Fatalf("Send through marketing: %v", err)
	}
	if err := queue.Send(ctx, "billing", "a@example.com", "Hello", "Body"); err == nil {
		t.Error("Send through an unconfigured name = nil, want the refusal")
	}

	want := []mailstdlib.Message{
		{Server: "default", Recipient: "a@example.com", Subject: "Hello", Body: "Body"},
		{Server: "marketing", Recipient: "list@example.com", Subject: "News", Body: "Issue 1"},
	}
	got := queue.Messages()
	if len(got) != len(want) {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("message %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

// TestMemory_Next covers reading the queue as a queue: the oldest message
// first, and gone once read, so a test walking a sequence of deliveries does
// not index into a slice. The second return reports an empty queue, because a
// zero Message reads the same as one a script sent with empty strings.
func TestMemory_Next(t *testing.T) {
	queue := mailstdlib.NewMemory()
	ctx := context.Background()
	for _, subject := range []string{"first", "second"} {
		if err := queue.Send(ctx, "default", "a@example.com", subject, "Body"); err != nil {
			t.Fatalf("Send(%s): %v", subject, err)
		}
	}

	for _, subject := range []string{"first", "second"} {
		message, ok := queue.Next()
		if !ok {
			t.Fatalf("Next() reported an empty queue before %q", subject)
		}
		if message.Subject != subject {
			t.Errorf("Next() = %q, want %q", message.Subject, subject)
		}
	}

	if message, ok := queue.Next(); ok {
		t.Errorf("Next() = %#v on a drained queue, want the empty report", message)
	}
}

// TestMemory_Reset covers emptying the queue, which a test reusing one
// provider across cases does between them. The configured names survive it, so
// the provider still refuses what it refused before.
func TestMemory_Reset(t *testing.T) {
	queue := mailstdlib.NewMemory("default")
	if err := queue.Send(context.Background(), "default", "a@example.com", "Hello", "Body"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	queue.Reset()
	if got := queue.Messages(); len(got) != 0 {
		t.Errorf("messages = %#v after Reset, want none", got)
	}
	if _, ok := queue.Next(); ok {
		t.Error("Next() found a message after Reset")
	}
	if err := queue.Configured("default"); err != nil {
		t.Errorf("Configured(default) = %v after Reset, want the name still held", err)
	}
	if err := queue.Configured("billing"); err == nil {
		t.Error("Configured(billing) = nil after Reset, want the refusal still held")
	}
}
