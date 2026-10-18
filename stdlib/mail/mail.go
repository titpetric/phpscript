// Package mail registers the mail bindings: the mail() function and the Mail
// class, both of which send through a server the host configured and named.
//
// A script names a server. It does not spell one: no binding accepts a host, a
// username or a password, and none returns one back
// out of the object a binding returns. The credentials live in a
// model.MailProvider the host builds from its configuration, which reads them
// at the moment of a delivery and returns the outcome alone.
//
// SMTP is the protocol the package speaks, not the subject it models, so it
// names the conversation in provider.go and nothing a script types.
package mail

import (
	"context"
	"fmt"
	netmail "net/mail"

	"github.com/titpetric/phpscript/model"
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
	// through the host's "default" mail server, throwing when none is
	// configured; PHP's $additional_headers and $additional_params are not
	// accepted.
	rt.RegisterFunc("mail", func(ctx context.Context, recipient, subject, body string) error {
		return send(ctx, provider(rt), Message{Server: DefaultName, Recipient: recipient, Subject: subject, Body: body})
	})
}

// serverName reads the one optional argument `new Mail` takes.
//
// The argument is taken as any and not as a string so that a settings
// array is refused and never coerced. A
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
// external work and no internal step. The recipient and the subject are
// recorded; the body and its HTML are left out and only their size kept. A
// body is the one part of a delivery that is certain to be private, and a size
// answers the question a trace is opened to answer.
//
// The server is recorded by its configured name and never by
// its hostname. The name is what an operator reads the trace against, and it
// is all this side of the provider holds, because the interface exposes
// no part of a credential, hostname included.
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

// messageSender is a provider that delivers a whole Message, HTML part and
// Reply-To included. Provider and Memory are. model.MailProvider declares a
// plain-text Send only, and stays that way so a host's own provider keeps
// compiling; one without SendMessage is refused a message it cannot carry.
type messageSender interface {
	SendMessage(ctx context.Context, message Message) error
}

// replyAddress reads the Reply-To a script passed as one address, a display
// name allowed. CR and LF are stripped first, as from every header, so a
// smuggled header leaves one malformed address behind, which is refused and
// never sent. A bare address stays bare; one with a name is re-rendered, which
// quotes the name and encodes it when it is not ASCII.
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
// Both fields are unexported, so the object is opaque from PHP:
// a binding publishes its exported methods and fields, so $mail->password,
// var_dump, print_r, get_object_vars and json_encode all find nothing. The
// only thing it can do is send, and the only thing it knows is a name.
type Mail struct {
	name     string
	provider model.MailProvider
}

// Send sends an email through the server this client names. $body is the plain
// text; a non-empty $html travels beside it as the alternative a mail client
// prefers, and a non-empty $reply_to, one address, is where a reply goes. It
// throws, sending nothing, when $reply_to is not one address. The runtime
// injects the context, so a script calls
// `$mail->send($to, $subject, $body, $html, $reply_to)`, the last two optional.
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
