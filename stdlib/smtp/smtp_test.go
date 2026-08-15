package smtp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	smtpstdlib "github.com/titpetric/phpscript/stdlib/smtp"
)

func TestRegister(t *testing.T) {
	program, err := parser.Parse(`<?php
		mail("recipient@example.com", "Subject", "Body");
		echo strtoupper("registered");
	?>`)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	rt := runner.New(&output, runner.Options{})
	queue := smtpstdlib.NewMemory()
	stdlib.Register(rt)
	smtpstdlib.Register(rt, queue)
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	if output.String() != "REGISTERED" {
		t.Fatalf("output = %q, want %q", output.String(), "REGISTERED")
	}
	want := smtpstdlib.Message{Recipient: "recipient@example.com", Subject: "Subject", Body: "Body"}
	if messages := queue.Messages(); len(messages) != 1 || messages[0] != want {
		t.Fatalf("messages = %#v, want %#v", messages, []smtpstdlib.Message{want})
	}
}

func TestRegisterSMTP(t *testing.T) {
	program, err := parser.Parse(`<?php
		$smtp = new SMTP(array(
			"host" => "mail.example.com",
			"port" => 587,
			"username" => "noreply@example.com",
			"password" => "secret",
			"from" => "BlackSky HAL <noreply@example.com>"
		));
		$smtp->send("hello@example.com", "Contact request", "Body line");
		echo "sent";
	?>`)
	if err != nil {
		t.Fatal(err)
	}

	// The in-memory sender stands in for the mail server: `new SMTP` hands its
	// messages to the sender bound in the runtime context.
	queue := smtpstdlib.NewMemory()
	var output bytes.Buffer
	rt := runner.New(&output, runner.Options{})
	rt.SetContext(smtpstdlib.SenderContext(context.Background(), queue))
	stdlib.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if output.String() != "sent" {
		t.Fatalf("output = %q, want %q", output.String(), "sent")
	}

	want := smtpstdlib.Message{Recipient: "hello@example.com", Subject: "Contact request", Body: "Body line"}
	message, ok := queue.Next()
	if !ok || message != want {
		t.Fatalf("message = %#v (queued %v), want %#v", message, ok, want)
	}
	if messages := queue.Messages(); len(messages) != 0 {
		t.Fatalf("messages = %#v, want the queue drained", messages)
	}
}

func TestRegisterSMTPErrors(t *testing.T) {
	tests := map[string]struct {
		script string
		want   string
	}{
		"missing host": {
			script: `$smtp = new SMTP(array("from" => "noreply@example.com"));`,
			want:   "host is required",
		},
		"missing from": {
			script: `$smtp = new SMTP(array("host" => "mail.example.com"));`,
			want:   "from is required",
		},
		"unknown option": {
			script: `$smtp = new SMTP(array("host" => "mail.example.com", "sender" => "x"));`,
			want:   `unknown option "sender"`,
		},
		"invalid from": {
			script: `$smtp = new SMTP(array("host" => "mail.example.com", "from" => "Example <not an address>"));`,
			want:   "invalid from address",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			program, err := parser.Parse("<?php " + test.script + " ?>")
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			rt := runner.New(&output, runner.Options{})
			stdlib.Register(rt)
			err = rt.Run(program)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want it to contain %q", err, test.want)
			}
		})
	}
}
