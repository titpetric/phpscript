package mail

import (
	"crypto/tls"
	"fmt"
	netmail "net/mail"
	"strings"
)

// DefaultPort is the port a server that names none is reached on. It is the
// plain SMTP port; a submission host usually wants 587.
const DefaultPort = 25

// Config contains the connection and sender settings for one mail server.
//
// It is host configuration and never reaches a script: a provider holds it,
// reads it inside a delivery, and hands back nothing but the outcome. Config
// is not a binding return type anywhere, which is what keeps a password out of
// the runtime scope.
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

// String renders the settings that identify a server and elides the two that
// authenticate to it.
//
// Nothing formats a Config today. The method is here so that nothing can start
// to by accident: a %v in an error or a log line is the cheapest way for a
// password to escape the process, and this makes that spelling harmless
// permanently. It does not affect the yaml decoder, which reads the fields.
func (c Config) String() string {
	auth := "no"
	if c.Username != "" || c.Password != "" {
		auth = "yes"
	}
	return fmt.Sprintf("smtp://%s:%d from=%q auth=%s insecure=%t", c.Host, c.Port, c.From, auth, c.Insecure)
}

// Validate reports whether the server is usable, naming it.
//
// Host and From are the operator's to get right, so they are checked when the
// configuration is read and fail the server at startup. Leaving it to the
// first delivery means finding out from a @schedule job at three in the
// morning, on the one code path nobody is watching.
func (c Config) Validate(name string) error {
	if c.Host == "" {
		return fmt.Errorf("mail %q: host is required", name)
	}
	if c.From == "" {
		return fmt.Errorf("mail %q: from is required", name)
	}
	if _, err := envelopeAddress(c.From); err != nil {
		return fmt.Errorf("mail %q: %w", name, err)
	}
	return nil
}

// address is the host:port the server is dialed on.
func (c Config) address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// tlsConfig returns the STARTTLS settings for the configured host. Insecure
// turns off verification entirely: the certificate is still used to encrypt the
// session, but neither its chain nor its names are checked, so the connection
// is no longer protected against a man in the middle.
func (c Config) tlsConfig() *tls.Config {
	return &tls.Config{
		ServerName:         c.Host,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: c.Insecure, //nolint:gosec // opt-in, see Config.Insecure
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
	address, err := netmail.ParseAddress(from)
	if err != nil {
		return "", fmt.Errorf("invalid from address %q: %w", from, err)
	}
	return address.Address, nil
}

// headerReplacer is built once rather than per header per message.
var headerReplacer = strings.NewReplacer("\r", "", "\n", "")

// header strips CR/LF so script-supplied values cannot inject headers.
func header(value string) string {
	return headerReplacer.Replace(value)
}
