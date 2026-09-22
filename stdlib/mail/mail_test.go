package mail_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	mailstdlib "github.com/titpetric/phpscript/stdlib/mail"
)

// run executes source on a runtime resolving through provider, returning what
// the script printed.
func run(t *testing.T, provider *mailstdlib.Memory, source string) string {
	t.Helper()

	program, err := parser.Parse("<?php " + source + " ?>")
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	options := runner.Options{}
	if provider != nil {
		options.Mail = provider
	}
	rt := runner.New(&output, options)
	stdlib.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

// TestMail covers the function against the host's default server.
func TestMail(t *testing.T) {
	queue := mailstdlib.NewMemory()
	got := run(t, queue, `
		mail("recipient@example.com", "Subject", "Body");
		echo strtoupper("registered");
	`)
	if got != "REGISTERED" {
		t.Fatalf("output = %q, want %q", got, "REGISTERED")
	}

	want := mailstdlib.Message{
		Server:    "default",
		Recipient: "recipient@example.com",
		Subject:   "Subject",
		Body:      "Body",
	}
	if messages := queue.Messages(); len(messages) != 1 || messages[0] != want {
		t.Fatalf("messages = %#v, want %#v", messages, []mailstdlib.Message{want})
	}
}

// TestMailClass covers the class: `new Mail` is the default server and
// `new Mail($name)` is the one the host configured under that name. Neither
// spelling carries a credential.
func TestMailClass(t *testing.T) {
	queue := mailstdlib.NewMemory("default", "marketing")
	got := run(t, queue, `
		$mail = new Mail;
		$mail->send("hello@example.com", "Contact request", "Body line");
		$campaigns = new Mail("marketing");
		$campaigns->send("list@example.com", "Newsletter", "Issue 1");
		echo "sent";
	`)
	if got != "sent" {
		t.Fatalf("output = %q, want %q", got, "sent")
	}

	want := []mailstdlib.Message{
		{Server: "default", Recipient: "hello@example.com", Subject: "Contact request", Body: "Body line"},
		{Server: "marketing", Recipient: "list@example.com", Subject: "Newsletter", Body: "Issue 1"},
	}
	messages := queue.Messages()
	if len(messages) != len(want) {
		t.Fatalf("messages = %#v, want %#v", messages, want)
	}
	for i := range want {
		if messages[i] != want[i] {
			t.Fatalf("message %d = %#v, want %#v", i, messages[i], want[i])
		}
	}
}

// TestMailUnknownServer pins the fail-fast: a name nobody configured is
// reported at construction, before a script has written a message, and the
// error names only the server that was asked for.
func TestMailUnknownServer(t *testing.T) {
	queue := mailstdlib.NewMemory("default")
	got := run(t, queue, `
		try {
			$mail = new Mail("marketing");
			echo "constructed";
		} catch (Exception $e) {
			echo $e->getMessage();
		}
	`)
	if want := "no configuration found for mail server: marketing"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// TestMailTakesAName pins the removal of the options-array constructor. It was
// the one way to put a password into the runtime scope, so the removal is a
// stated behaviour rather than an omission.
func TestMailTakesAName(t *testing.T) {
	got := run(t, mailstdlib.NewMemory(), `
		try {
			$mail = new Mail(array("host" => "mail.example.com", "password" => "secret"));
			echo "constructed";
		} catch (Throwable $e) {
			echo $e->getMessage();
		}
	`)
	if !strings.Contains(got, "not the settings of one") {
		t.Fatalf("output = %q, want it to refuse the settings array", got)
	}
}

// TestMailUnconfigured covers the default provider: mail() and `new Mail`
// exist on every runtime and refuse catchably, so calling code keeps one
// spelling and its own fallback.
func TestMailUnconfigured(t *testing.T) {
	got := run(t, nil, `
		echo function_exists("mail") ? "exists" : "missing";
		try {
			mail("a@example.com", "s", "b");
			echo "|sent";
		} catch (Exception $e) {
			echo "|" . $e->getMessage();
		}
	`)
	if want := "exists|no configuration found for mail server: default"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// TestMailExposesNoCredentialToScripts is the invariant the whole binding
// exists to hold: a script that holds a configured server, password and all,
// has no spelling that gets the password back out.
func TestMailExposesNoCredentialToScripts(t *testing.T) {
	queue := mailstdlib.NewMemory("marketing")
	got := run(t, queue, `
		$mail = new Mail("marketing");
		echo json_encode($mail);
		echo "|" . count(get_object_vars($mail));
		echo "|" . ($mail->password === null ? "null" : "readable");
		echo "|" . ($mail->host === null ? "null" : "readable");
		echo "|" . ($mail->name === null ? "null" : "readable");
	`)
	if want := "{}|0|null|null|null"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("output = %q, want no credential in it", got)
	}
}
