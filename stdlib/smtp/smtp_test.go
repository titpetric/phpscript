package smtp_test

import (
	"bytes"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	smtpstdlib "github.com/titpetric/phpscript/stdlib/smtp"
)

type message struct {
	recipient string
	subject   string
	body      string
}

type sender struct {
	messages []message
}

func (s *sender) Send(recipient, subject, body string) error {
	s.messages = append(s.messages, message{recipient, subject, body})
	return nil
}

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
	s := new(sender)
	smtpstdlib.Register(rt, s)
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	if output.String() != "REGISTERED" {
		t.Fatalf("output = %q, want %q", output.String(), "REGISTERED")
	}
	want := message{"recipient@example.com", "Subject", "Body"}
	if len(s.messages) != 1 || s.messages[0] != want {
		t.Fatalf("messages = %#v, want %#v", s.messages, []message{want})
	}
}
