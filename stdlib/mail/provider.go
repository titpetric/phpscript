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
// it resolved to. It is the one function a host swaps to send by something other than
// SMTP, and it is a function and no second MailProvider because the name
// resolution and the tenant isolation sit above it and are not to be
// reimplemented. A Deliver is host code and reads the host's settings.
type Deliver func(config Config, recipient, subject, body string) error

// Provider holds the credentials of a set of named mail servers.
//
// The map is fixed once the provider is built. There is no
// registration call and no listing call: a script names a server and learns
// only whether the delivery worked. That is also what lets the constructor
// check a name and get the same verdict Send
// makes for itself.
type Provider struct {
	servers map[string]Config
	deliver Deliver
}

// NewProvider returns a provider holding the given servers, keyed by the name
// a script names, sending over SMTP.
func NewProvider(servers map[string]Config) *Provider {
	return NewProviderFunc(servers, deliverSMTP)
}

// NewProviderFunc is NewProvider with the transport replaced, the way
// database.NewDatabaseProvider takes its connector. A nil deliver is SMTP.
//
// The map is copied and never retained, because configuration layering passes
// one map to every site that declared no mail block, and a normalisation
// written into it
// would be written into all of them. Names are lowercased and a missing port is
// filled in here, so a delivery reads a complete credential.
func NewProviderFunc(servers map[string]Config, deliver Deliver) *Provider {
	if deliver == nil {
		deliver = deliverSMTP
	}
	result := &Provider{
		servers: make(map[string]Config, len(servers)),
		deliver: deliver,
	}
	for name, config := range servers {
		if config.Port == 0 {
			config.Port = DefaultPort
		}
		result.servers[strings.ToLower(name)] = config
	}
	return result
}

// Configured reports why name cannot be delivered through, and nil when it can.
func (p *Provider) Configured(name string) error {
	_, err := p.resolve(name)
	return err
}

// Send sends one message through the named server.
//
// The credential is read here, into a local, and is gone when the call
// returns. Nothing that outlives the delivery holds it: not the PHP object the
// script called, and no part of the span or the error.
func (p *Provider) Send(_ context.Context, name, recipient, subject, body string) error {
	config, err := p.resolve(name)
	if err != nil {
		return err
	}
	return p.deliver(config, recipient, subject, body)
}

// resolve returns the credential for name, defaulting an empty name. A name
// nobody configured is reported the way an unconfigured database connection
// is: the name named, and nothing about what was configured.
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
// the host named none. It follows smtp.SendMail, which cannot be
// used directly because it builds its own TLS config, leaving no way to reach a
// host whose certificate does not verify (see Config.Insecure).
func deliverSMTP(config Config, recipient, subject, body string) error {
	sender, err := envelopeAddress(config.From)
	if err != nil {
		return err
	}
	message := buildMessage(config.From, recipient, subject, body)

	// A host that wants no authentication, mailhog being the usual one,
	// configures neither a username nor a password.
	var auth smtp.Auth
	if config.Username != "" && config.Password != "" {
		auth = smtp.PlainAuth("", config.Username, config.Password, config.Host)
	}

	if err := converse(config, auth, sender, recipient, message); err != nil {
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
