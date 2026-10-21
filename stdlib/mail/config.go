package mail

import (
	"crypto/tls"
	"fmt"
	netmail "net/mail"
	"strings"
)

// DefaultPort is the port a provider fills in for a server that names none, the
// plain SMTP port; a submission host listens on 587.
const DefaultPort = 25

// Config contains the connection and sender settings for one mail server.
//
// Config is host configuration and never reaches a script: a provider holds
// it, reads it inside a delivery, and returns the outcome alone. Config is no
// binding return type anywhere, which keeps a password out of the runtime
// scope.
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
// authenticate to it, so a %v in a log line leaks no password.
func (c Config) String() string {
	auth := "no"
	if c.Username != "" || c.Password != "" {
		auth = "yes"
	}
	return fmt.Sprintf("smtp://%s:%d from=%q auth=%s insecure=%t", c.Host, c.Port, c.From, auth, c.Insecure)
}

// Check reports whether the server is usable, naming it.
//
// Host and From are the operator's to get right, so the configuration check
// tests them at startup and fails the server there. Leaving it to the first
// delivery means finding out from a @schedule job at three in the morning, on
// the one code path nobody is watching.
func (c Config) Check(name string) error {
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

// address is the host:port a delivery dials the server on.
func (c Config) address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// tlsConfig returns the STARTTLS settings for the configured host. Insecure
// turns off verification entirely: the certificate is still used to encrypt the
// session, but the dial verifies neither its chain nor its names, so the
// connection is open to a man in the middle.
func (c Config) tlsConfig() *tls.Config {
	return &tls.Config{
		ServerName:         c.Host,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: c.Insecure, //nolint:gosec // opt-in, see Config.Insecure
	}
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
