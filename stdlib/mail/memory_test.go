package mail_test

import (
	"context"
	"testing"

	mailstdlib "github.com/titpetric/phpscript/stdlib/mail"
)

// TestMemory_Configured covers which names an in-memory provider accepts.
// Naming none configures every name and a dry run captures whatever a script
// sends; naming some lets a test read back the refusal a script reads for the
// rest. An empty name is the "default" server, as everywhere else.
func TestMemory_Configured(t *testing.T) {
	t.Parallel()
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
// carries its target server's name and a test reads back which server a script
// selected. This provider refuses a name it does not hold the way a real
// delivery refuses it and a test still exercises the error a script reads.
func TestMemory_Send(t *testing.T) {
	t.Parallel()
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

// TestMemory_SendMessage covers queueing a whole message: the queue holds the
// HTML part and the Reply-To as sent, an empty server is the "default" one, and
// this provider refuses a name it does not hold the way a real delivery does.
func TestMemory_SendMessage(t *testing.T) {
	t.Parallel()
	queue := mailstdlib.NewMemory("default")
	ctx := context.Background()

	sent := mailstdlib.Message{Recipient: "a@example.com", Subject: "Hello", Body: "Body", HTML: "<p>Body</p>", ReplyTo: "lead@example.com"}
	if err := queue.SendMessage(ctx, sent); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if err := queue.SendMessage(ctx, mailstdlib.Message{Server: "billing", Recipient: "a@example.com"}); err == nil {
		t.Error("SendMessage through an unconfigured name = nil, want the refusal")
	}

	want := sent
	want.Server = "default"
	if got := queue.Messages(); len(got) != 1 || got[0] != want {
		t.Fatalf("messages = %#v, want %#v", got, []mailstdlib.Message{want})
	}
}

// TestMemory_Next covers reading the queue as a queue: the oldest message
// first, and gone once read and a test walking a sequence of deliveries does
// not index into a slice. The second return reports an empty queue, because a
// zero Message reads the same as one a script sent with empty strings.
func TestMemory_Next(t *testing.T) {
	t.Parallel()
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
// provider across cases does between them. The configured names survive it and
// the provider still refuses what it refused before.
func TestMemory_Reset(t *testing.T) {
	t.Parallel()
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
