// Package mail registers the mail bindings: the mail() function and the Mail
// class, both of which deliver through a server the host configured and named.
//
// A script names a server. It does not spell one: there is no way to hand a
// host, a username or a password to a binding, and no way to read one back
// out of the object a binding returns. The credentials live in a
// model.MailProvider the host builds from its configuration, which reads them
// at the moment of a delivery and hands back nothing but the outcome.
//
// SMTP is the protocol the package speaks, not the subject it models, so it
// names the conversation in provider.go and nothing a script types.
package mail

import (
	"context"
	"fmt"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

// Register installs the mail bindings on rt.
func Register(rt *runner.Runtime) {
	// Mail delivers through one of the mail servers the host configured,
	// selected by $name; `new Mail` selects "default". The credentials stay
	// with the host: a script names a server and never spells, or reads back,
	// a host, a username or a password. The constructor throws when $name is
	// not a configured server.
	//
	//	$mail = new Mail;              // the "default" server
	//	$mail = new Mail("marketing"); // a server the host named
	//	$mail->send("hello@example.com", "Subject", "Body");
	rt.RegisterConstructor("Mail", func(names ...any) (*Mail, error) {
		name, err := serverName(names)
		if err != nil {
			return nil, err
		}
		servers := provider(rt)
		if err := servers.Configured(name); err != nil {
			return nil, err
		}
		return &Mail{name: name, provider: servers}, nil
	})

	// mail sends a plain-text message to $recipient with $subject and $body through the host's "default" mail server, throwing when none is configured; PHP's $additional_headers and $additional_params are not accepted.
	rt.RegisterFunc("mail", func(ctx context.Context, recipient, subject, body string) error {
		return send(ctx, provider(rt), DefaultName, recipient, subject, body)
	})
}

// serverName reads the one optional argument `new Mail` takes.
//
// The argument is taken as any rather than as a string so that the settings
// array the constructor used to accept is refused rather than coerced. A
// string parameter turns that array into the empty name, which resolves to
// "default": a script written against the old binding would go on sending,
// silently, through whatever server the host configured first. It has to fail,
// and it has to say where the settings live now.
func serverName(names []any) (string, error) {
	if len(names) > 1 {
		return "", fmt.Errorf("Mail(): expects at most 1 argument, %d given", len(names))
	}
	if len(names) == 0 || names[0] == nil {
		return DefaultName, nil
	}

	name, ok := names[0].(string)
	if !ok {
		if model.IsCollection(names[0]) {
			return "", fmt.Errorf("Mail(): argument #1 ($name) must be the name of a configured server, not the settings of one: connection settings are the host's, and belong in the mail block of its configuration")
		}
		return "", fmt.Errorf("Mail(): argument #1 ($name) must be of type string")
	}
	if name == "" {
		return DefaultName, nil
	}
	return name, nil
}

// send is the one delivery path both bindings take, so every message is
// recorded exactly once however the script spelled it.
//
// Mail leaves the process and is the slowest thing most scripts do, so it is
// external work rather than internal. The recipient and the subject are
// recorded; the body is not, only its size. A message body is the one part of
// a delivery that is certain to be private, and a size answers the question a
// trace is opened to answer.
//
// The server is recorded by the name it was configured under rather than by
// its hostname. The name is what an operator reads the trace against, and it
// is the only thing this side of the provider knows: the interface hands out
// no part of a credential, hostname included.
func send(ctx context.Context, servers model.MailProvider, name, recipient, subject, body string) (err error) {
	span := telemetry.StartSpan(ctx, "mail", telemetry.KindExternal)
	span.SetAttribute("server", name)
	span.SetAttribute("to", recipient)
	span.SetAttribute("subject", subject)
	span.SetAttribute("bytes", len(body))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return servers.Send(ctx, name, recipient, subject, body)
}

// Mail is the handle a script holds on one configured server.
//
// Both fields are unexported, which is what makes the object opaque from PHP:
// a binding publishes its exported methods and fields, so $mail->password,
// var_dump, print_r, get_object_vars and json_encode all find nothing. The
// only thing it can do is send, and the only thing it knows is a name.
type Mail struct {
	name     string
	provider model.MailProvider
}

// Send sends an email through the server this client names. It takes a context
// so the delivery is recorded on the trace of the request that asked for it;
// the runtime injects it, so a script still calls
// `$mail->send($to, $subject, $body)`.
func (m *Mail) Send(ctx context.Context, recipient, subject, body string) error {
	return send(ctx, m.provider, m.name, recipient, subject, body)
}
