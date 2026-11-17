// Package mail registers the mail bindings: the mail() function and the Mail
// class, both of which send through a server the host configured and named.
//
// A script names a server. It does not spell one: no binding accepts a host, a
// username or a password, and no method reads one back out of the object a
// binding returns. A model.MailProvider the host builds from its configuration
// holds the credentials, reads them at the moment of a delivery and returns
// the outcome alone.
//
// SMTP is the protocol the package speaks, not the subject it models and it
// names the conversation in provider.go and nothing a script types.
package mail

import (
	"context"
	"fmt"
	netmail "net/mail"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

// Register installs the mail bindings on rt.
func Register(rt *runner.Runtime) {
	// Mail sends through one of the mail servers the host configured,
	// selected by $name; `new Mail` selects "default". The credentials stay
	// with the host: a script names a server and never spells, or reads back,
	// a host, a username or a password. The constructor throws when $name is
	// not a configured server.
	//
	//	$mail = new Mail;              // the "default" server
	//	$mail = new Mail("marketing"); // a server the host named
	//	$mail->send("hello@example.com", "Subject", "Body");
	//	$mail->send("hello@example.com", "Subject", "Body", "<p>Body</p>", "support@example.com");
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

	// mail sends a plain-text message to $recipient with $subject and $body
	// through the host's "default" mail server, throwing when the host
	// configured none; PHP's $additional_headers and $additional_params are not
	// accepted.
	rt.RegisterFunc("mail", func(ctx context.Context, recipient, subject, body string) error {
		return send(ctx, provider(rt), Message{Server: DefaultName, Recipient: recipient, Subject: subject, Body: body})
	})
}

// serverName reads `new Mail`'s optional name and refuses a settings array.
func serverName(names []any) (string, error) {
	if len(names) > 1 {
		return "", fmt.Errorf("Mail(): expects at most 1 argument, %d given", len(names))
	}
	if len(names) == 0 || names[0] == nil {
		return DefaultName, nil
	}

	name, ok := names[0].(string)
	if !ok {
		if modelutil.IsCollection(names[0]) {
			return "", fmt.Errorf("Mail(): argument #1 ($name) must be the name of a configured server, not the settings of one: connection settings are the host's, and belong in the mail block of its configuration")
		}
		return "", fmt.Errorf("Mail(): argument #1 ($name) must be of type string")
	}
	if name == "" {
		return DefaultName, nil
	}
	return name, nil
}

// send delivers a message and records its recipient, subject and size once on the span.
func send(ctx context.Context, servers model.MailProvider, message Message) (err error) {
	span := telemetry.StartSpan(ctx, "mail", telemetry.KindExternal)
	span.SetAttribute("server", message.Server)
	span.SetAttribute("to", message.Recipient)
	span.SetAttribute("subject", message.Subject)
	span.SetAttribute("bytes", len(message.Body)+len(message.HTML))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if sender, ok := servers.(messageSender); ok {
		return sender.SendMessage(ctx, message)
	}
	if message.HTML != "" || message.ReplyTo != "" {
		return errPlainTransport
	}
	return servers.Send(ctx, message.Server, message.Recipient, message.Subject, message.Body)
}

// messageSender is the optional provider interface delivering a whole Message.
type messageSender interface {
	SendMessage(ctx context.Context, message Message) error
}

// replyAddress reads one Reply-To address, refusing a value that smuggled a header.
func replyAddress(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	address, err := netmail.ParseAddress(header(value))
	if err != nil {
		// %v and not %w: a catch takes the innermost error of a chain, which
		// would leave the script net/mail's wording without the argument.
		return "", fmt.Errorf("Mail::send(): argument #5 ($reply_to) must be one email address: %v", err)
	}
	if address.Name == "" {
		return address.Address, nil
	}
	return address.String(), nil
}

// Mail is the handle a script holds on one configured server.
//
// Both fields stay unexported and the object is opaque from PHP: a binding
// publishes its exported methods and fields and $mail->password, var_dump,
// print_r, get_object_vars and json_encode all find nothing. The only thing it
// can do is send, and the only thing it knows is a name.
type Mail struct {
	name     string
	provider model.MailProvider
}

// Send sends an email through the server this client names, as
// `$mail->send($to, $subject, $body, $html, $reply_to)` with the last two
// optional. It throws, sending nothing, when $reply_to is not one address.
func (m *Mail) Send(ctx context.Context, recipient, subject, body, html, reply_to string) error {
	replyTo, err := replyAddress(reply_to)
	if err != nil {
		return err
	}
	return send(ctx, m.provider, Message{
		Server:    m.name,
		Recipient: recipient,
		Subject:   subject,
		Body:      body,
		HTML:      html,
		ReplyTo:   replyTo,
	})
}
