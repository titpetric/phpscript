package mail_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
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

// TestRegister covers the surface the installer puts within reach: the Mail
// class and the mail function, each under the name a script types. The
// installer adds both with or without a configured server, so calling code
// keeps one spelling and its own fallback.
func TestRegister(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	mailstdlib.Register(rt)

	if _, ok := rt.LookupConstructor("Mail"); !ok {
		t.Error("new Mail resolves to nothing after Register")
	}
	if _, ok := rt.LookupFunc("mail"); !ok {
		t.Error("mail() resolves to nothing after Register")
	}
}

// TestMail_Send covers the deliveries a held client makes. The object carries
// a server name and nothing else, so each send resolves the credential afresh
// inside the provider; one handle sends any number of messages, and every one
// of them records the server the handle named.
func TestMail_Send(t *testing.T) {
	t.Parallel()
	queue := mailstdlib.NewMemory("marketing")
	got := run(t, queue, `
		$mail = new Mail("marketing");
		$mail->send("list@example.com", "Newsletter", "Issue 1");
		$mail->send("list@example.com", "Newsletter", "Issue 2");
		echo "sent";
	`)
	if got != "sent" {
		t.Fatalf("output = %q, want %q", got, "sent")
	}

	messages := queue.Messages()
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want two", messages)
	}
	for i, want := range []string{"Issue 1", "Issue 2"} {
		if messages[i].Body != want || messages[i].Server != "marketing" {
			t.Errorf("message %d = %#v, want %q through marketing", i, messages[i], want)
		}
	}
}

// TestMail covers the function against the host's default server.
func TestMail(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// TestMail_SendHTML covers the two optional arguments of send(): $html travels
// beside the text and $reply_to becomes the Reply-To, which reaches the
// provider as one address. A bare address stays bare, and a display name comes
// back quoted, the way buildMessage writes it into the header.
func TestMail_SendHTML(t *testing.T) {
	t.Parallel()
	queue := mailstdlib.NewMemory()
	got := run(t, queue, `
		$mail = new Mail;
		$mail->send("a@example.com", "Week sent back", "Open it to see why.", "<p>Open it to see why.</p>", "Team Lead <lead@example.com>");
		$mail->send("b@example.com", "Invitation", "Join us.", "", "lead@example.com");
		$mail->send("c@example.com", "Sign-in code", "", "<p>123456</p>");
		echo "sent";
	`)
	if got != "sent" {
		t.Fatalf("output = %q, want %q", got, "sent")
	}

	want := []mailstdlib.Message{
		{Server: "default", Recipient: "a@example.com", Subject: "Week sent back", Body: "Open it to see why.", HTML: "<p>Open it to see why.</p>", ReplyTo: `"Team Lead" <lead@example.com>`},
		{Server: "default", Recipient: "b@example.com", Subject: "Invitation", Body: "Join us.", ReplyTo: "lead@example.com"},
		{Server: "default", Recipient: "c@example.com", Subject: "Sign-in code", HTML: "<p>123456</p>"},
	}
	messages := queue.Messages()
	if len(messages) != len(want) {
		t.Fatalf("messages = %#v, want %#v", messages, want)
	}
	for i := range want {
		if messages[i] != want[i] {
			t.Errorf("message %d = %#v, want %#v", i, messages[i], want[i])
		}
	}
}

// TestMail_SendReplyToInjection pins the refusal of a Reply-To that is not one
// address. A header smuggled in behind a line break leaves a malformed address
// once header strips the break, so the call throws and nothing reaches the
// provider; send refuses a list too, because the argument names one address.
func TestMail_SendReplyToInjection(t *testing.T) {
	t.Parallel()
	for _, replyTo := range []string{
		`lead@example.com\r\nBcc: everyone@example.net`,
		`lead@example.com\nBcc: everyone@example.net`,
		`Lead <lead@example.com>\r\nBcc: everyone@example.net`,
		`lead@example.com, everyone@example.net`,
		`not an address`,
	} {
		t.Run(replyTo, func(t *testing.T) {
			t.Parallel()
			queue := mailstdlib.NewMemory()
			got := run(t, queue, `
				$mail = new Mail;
				try {
					$mail->send("a@example.com", "Subject", "Text", "<p>HTML</p>", "`+replyTo+`");
					echo "sent";
				} catch (Exception $e) {
					echo $e->getMessage();
				}
			`)
			if !strings.HasPrefix(got, "Mail::send(): argument #5 ($reply_to) must be one email address") {
				t.Fatalf("output = %q, want the refusal", got)
			}
			if messages := queue.Messages(); len(messages) != 0 {
				t.Fatalf("messages = %#v, want none", messages)
			}
		})
	}
}

// plainProvider is a model.MailProvider with Send and no SendMessage, the shape
// a host's own provider written before HTML had.
type plainProvider struct{ model.MailProvider }

// TestMail_SendThroughAPlainProvider covers a provider that declares the
// interface and nothing more. Its Send still delivers plain text; the binding
// refuses a message with an HTML part or a Reply-To, because delivering the
// text alone would send a different message than the script wrote.
func TestMail_SendThroughAPlainProvider(t *testing.T) {
	t.Parallel()
	queue := mailstdlib.NewMemory()
	var output bytes.Buffer
	program, err := parser.Parse(`<?php
		$mail = new Mail;
		$mail->send("a@example.com", "Plain", "Text");
		foreach (array(array("<p>HTML</p>", ""), array("", "lead@example.com")) as $extra) {
			try {
				$mail->send("a@example.com", "Rich", "Text", $extra[0], $extra[1]);
				echo "sent\n";
			} catch (Exception $e) {
				echo $e->getMessage(), "\n";
			}
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
	rt := runner.New(&output, runner.Options{Mail: plainProvider{queue}})
	stdlib.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	refusal := "mail: this transport sends plain text only, so a message with an HTML part or a Reply-To cannot be delivered\n"
	if got := output.String(); got != refusal+refusal {
		t.Fatalf("output = %q, want the refusal twice", got)
	}
	want := mailstdlib.Message{Server: "default", Recipient: "a@example.com", Subject: "Plain", Body: "Text"}
	if messages := queue.Messages(); len(messages) != 1 || messages[0] != want {
		t.Fatalf("messages = %#v, want %#v", messages, []mailstdlib.Message{want})
	}
}

// TestMailUnknownServer pins the fail-fast: the constructor reports a name
// nobody configured before a script has written a message, and the error
// names the server the script selects, and no other.
func TestMailUnknownServer(t *testing.T) {
	t.Parallel()
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

// TestMailTakesAName pins the absence of an options-array constructor. One
// would be the single way to put a password into the runtime scope, so its
// absence is a stated behaviour and no omission.
func TestMailTakesAName(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
