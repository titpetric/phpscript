package mail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/smtp"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// DefaultName is the server a script that names none resolves to. `new Mail` and
// mail() both resolve it.
const DefaultName = "default"

// Default is the provider used when a runtime's options name none. It holds no
// servers, so mail() and `new Mail` refuse catchably naming what is missing,
// as a CLI run with no mail block has.
var Default model.MailProvider = NewProvider(nil)

var _ model.MailProvider = (*Provider)(nil)

// Deliver passes one message to a transport, with the credential of the server
// it resolved to. A host swaps this one function to send by something other
// than SMTP. It stays a function and no second MailProvider because the name
// resolution and the tenant isolation sit above it, where no host rewrites
// them. A Deliver is host code and reads the host's settings.
type Deliver func(config Config, recipient, subject, body string) error

// errPlainTransport refuses a message a plain-text transport would have to cut
// down: dropping the HTML or the Reply-To would deliver a different message.
var errPlainTransport = errors.New("mail: this transport sends plain text only, so a message with an HTML part or a Reply-To cannot be delivered")

// plain passes a message to a transport that reads the three parts of a
// plain-text one.
func (deliver Deliver) plain(config Config, message Message) error {
	if message.HTML != "" || message.ReplyTo != "" {
		return errPlainTransport
	}
	return deliver(config, message.Recipient, message.Subject, message.Body)
}

// Provider holds the credentials of a set of named mail servers.
//
// Nothing changes the map after the constructor fills it. There is no
// registration call and no listing call: a script names a server and learns
// only whether the delivery worked. That is also what lets the constructor
// check a name and get the same verdict Send makes for itself.
type Provider struct {
	servers map[string]Config
	deliver func(Config, Message) error
}

// NewProvider returns a provider holding the given servers, keyed by the name
// a script names, sending over SMTP.
func NewProvider(servers map[string]Config) *Provider {
	return NewProviderFunc(nil, servers)
}

// NewProviderFunc is NewProvider with the transport replaced, the way
// database.NewDatabaseProvider takes its connector. A nil deliver is SMTP. A
// Deliver reads a plain-text message, so plain refuses a message with an HTML
// part or a Reply-To: cut down to fit, it would be a different message.
//
// The constructor copies the map and keeps no reference, because configuration
// layering passes one map to every site that declared no mail block, and a
// normalisation written into it would reach every one. It lowercases a name
// and fills in a missing port, so a delivery reads a complete credential.
func NewProviderFunc(deliver Deliver, servers map[string]Config) *Provider {
	result := &Provider{
		servers: make(map[string]Config, len(servers)),
		deliver: deliverSMTP,
	}
	if deliver != nil {
		result.deliver = deliver.plain
	}
	for name, config := range servers {
		if config.Port == 0 {
			config.Port = DefaultPort
		}
		result.servers[strings.ToLower(name)] = config
	}
	return result
}

// Configured reports why name cannot carry a delivery, and nil when it can.
func (p *Provider) Configured(name string) error {
	_, err := p.resolve(name)
	return err
}

// Send sends one plain-text message through the named server.
func (p *Provider) Send(ctx context.Context, name, recipient, subject, body string) error {
	return p.SendMessage(ctx, Message{Server: name, Recipient: recipient, Subject: subject, Body: body})
}

// SendMessage sends message through the server it names.
//
// It reads the credential into a local and drops it when the call returns.
// Nothing that outlives the delivery holds it: not the PHP object the script
// called, and no part of the span or the error.
func (p *Provider) SendMessage(_ context.Context, message Message) error {
	config, err := p.resolve(message.Server)
	if err != nil {
		return err
	}
	return p.deliver(config, message)
}

// resolve returns the credential for name, defaulting an empty name. For a
// name nobody configured, the error carries what an unconfigured database
// connection carries: the name the caller passes, and nothing about the
// configuration.
func (p *Provider) resolve(name string) (Config, error) {
	if name == "" {
		name = DefaultName
	}
	config, ok := p.servers[strings.ToLower(name)]
	if !ok {
		return Config{}, fmt.Errorf("no configuration found for mail server: %s", name)
	}
	return config, nil
}

// deliverSMTP runs the SMTP conversation, the transport a provider uses when
// the host named none. It follows smtp.SendMail, which nothing here can call
// directly because it builds its own TLS configuration, leaving no way to
// reach a host whose certificate does not verify (see Config.Insecure).
func deliverSMTP(config Config, message Message) error {
	sender, err := envelopeAddress(config.From)
	if err != nil {
		return err
	}
	rendered, err := buildMessage(config.From, message)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	// A host that wants no authentication, mailhog being the usual one,
	// configures neither a username nor a password.
	var auth smtp.Auth
	if config.Username != "" && config.Password != "" {
		auth = smtp.PlainAuth("", config.Username, config.Password, config.Host)
	}

	if err := converse(config, auth, sender, message.Recipient, rendered); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}

// converse is the SMTP exchange itself, split out so deliver reads as the
// decisions and this one reads as the protocol.
func converse(config Config, auth smtp.Auth, sender, recipient, message string) error {
	client, err := smtp.Dial(config.address())
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(config.tlsConfig()); err != nil {
			return err
		}
	}

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); !ok {
			return errors.New("smtp: server doesn't support AUTH")
		}
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(sender); err != nil {
		return err
	}
	if err := client.Rcpt(recipient); err != nil {
		return err
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(writer, message); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	return client.Quit()
}

// provider returns the mail servers rt resolves through. A host that configured
// none, which is every CLI run without a mail block, gets Default, which holds
// no servers at all.
func provider(rt *runner.Runtime) model.MailProvider {
	if configured := rt.Mail(); configured != nil {
		return configured
	}
	return Default
}
