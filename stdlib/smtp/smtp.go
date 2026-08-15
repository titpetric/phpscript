package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"net/smtp"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

type senderKey struct{}

// SenderContext binds a Sender into the context. A runtime running with that
// context constructs `new SMTP` clients that hand their messages to the sender
// rather than dialing the configured host, which is how a host captures mail
// (see Memory) without a mail server.
func SenderContext(ctx context.Context, sender Sender) context.Context {
	return context.WithValue(ctx, senderKey{}, sender)
}

// Config contains the connection and sender settings for SMTP.
type Config struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
	From     string `yaml:"from" json:"from"`

	// Insecure accepts the STARTTLS certificate without verifying its chain or
	// names. Hosts that present a self-signed certificate, or one carrying no
	// subjectAltName, are unreachable without it.
	Insecure bool `yaml:"insecure" json:"insecure"`
}

// Sender interface defines methods for sending emails
type Sender interface {
	Send(recipient, subject, body string) error
}

// Register binds mail() to sender. The standard library itself is registered
// separately by the embedding host (stdlib.Register).
func Register(rt *runner.Runtime, sender Sender) {
	rt.RegisterFunc("mail", sender.Send)
}

// RegisterSMTP installs the SMTP class binding, so `new SMTP` in a script
// constructs a script-configured sender.
func RegisterSMTP(rt *runner.Runtime) {
	rt.RegisterConstructor("SMTP", NewSMTPBinding)
}

// NewSMTPBinding is the constructor callback registered for SMTP. It takes the
// connection settings as an associative array:
//
//	$smtp = new SMTP(array(
//		"host"     => "mail.example.com",
//		"port"     => 587,
//		"username" => "noreply@example.com",
//		"password" => "secret",
//		"from"     => "Example <noreply@example.com>",
//		"insecure" => true,
//	));
func NewSMTPBinding(ctx context.Context, options *model.Array) (*SMTP, error) {
	if options == nil {
		return nil, fmt.Errorf("SMTP: connection options are required")
	}

	config := Config{Port: 25}
	var err error
	options.Range(func(key, value any) bool {
		switch strings.ToLower(toString(key)) {
		case "host":
			config.Host = toString(value)
		case "port":
			config.Port = toInt(value)
		case "username", "user":
			config.Username = toString(value)
		case "password", "pass":
			config.Password = toString(value)
		case "from":
			config.From = toString(value)
		case "insecure":
			config.Insecure = toBool(value)
		default:
			err = fmt.Errorf("SMTP: unknown option %q", toString(key))
			return false
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if config.Host == "" {
		return nil, fmt.Errorf("SMTP: host is required")
	}
	if config.From == "" {
		return nil, fmt.Errorf("SMTP: from is required")
	}
	if _, err := envelopeAddress(config.From); err != nil {
		return nil, err
	}
	client := NewSMTP(config)
	if sender, ok := ctx.Value(senderKey{}).(Sender); ok && sender != nil {
		client.sender = sender
	}
	return client, nil
}

// SMTP implements the Sender interface using SMTP
type SMTP struct {
	config Config

	// sender, when set, receives messages instead of the configured host.
	sender Sender
}

// NewSMTP creates a new SMTP client
func NewSMTP(config Config) *SMTP {
	return &SMTP{config: config}
}

// Send sends an email via SMTP
func (s *SMTP) Send(recipient, subject, body string) error {
	if s.sender != nil {
		return s.sender.Send(recipient, subject, body)
	}

	sender, err := envelopeAddress(s.config.From)
	if err != nil {
		return err
	}

	message := buildMessage(s.config.From, recipient, subject, body)

	// For mailhog, no authentication is needed
	var auth smtp.Auth
	if s.config.Username != "" && s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
	}

	// Send the email
	if err := s.deliver(auth, sender, recipient, message); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// deliver runs the SMTP conversation. It follows smtp.SendMail, which cannot be
// used directly because it builds its own TLS config, leaving no way to reach a
// host whose certificate does not verify (see Config.Insecure).
func (s *SMTP) deliver(auth smtp.Auth, sender, recipient, message string) error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(s.tlsConfig()); err != nil {
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

// tlsConfig returns the STARTTLS settings for the configured host. Insecure
// turns off verification entirely: the certificate is still used to encrypt the
// session, but neither its chain nor its names are checked, so the connection
// is no longer protected against a man in the middle.
func (s *SMTP) tlsConfig() *tls.Config {
	return &tls.Config{
		ServerName:         s.config.Host,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: s.config.Insecure, //nolint:gosec // opt-in, see Config.Insecure
	}
}

// buildMessage renders the RFC 5322 message. From carries the configured
// display name ("Name <user@host>"), the envelope sender is the bare address.
func buildMessage(from, recipient, subject, body string) string {
	headers := []string{
		"From: " + header(from),
		"To: " + header(recipient),
		"Subject: " + header(subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body
}

// envelopeAddress extracts the bare address from an address that may carry a
// display name, as required by the SMTP MAIL FROM command.
func envelopeAddress(from string) (string, error) {
	if !strings.ContainsAny(from, "<>") {
		return from, nil
	}
	address, err := mail.ParseAddress(from)
	if err != nil {
		return "", fmt.Errorf("SMTP: invalid from address %q: %w", from, err)
	}
	return address.Address, nil
}

// header strips CR/LF so script-supplied values cannot inject headers.
func header(value string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(value)
}

func toString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// toBool follows PHP truthiness for the values a script can hand an option:
// "false", "off", "no", "0" and the empty string are false, as is a zero
// number; anything else set is true.
func toBool(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "0", "false", "off", "no":
			return false
		}
		return true
	default:
		return toInt(value) != 0
	}
}

func toInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	default:
		return 0
	}
}
