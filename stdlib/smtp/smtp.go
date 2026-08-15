package smtp

import (
	"fmt"
	"net/smtp"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// Config contains the connection and sender settings for SMTP.
type Config struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
	From     string `yaml:"from" json:"from"`
}

// Sender interface defines methods for sending emails
type Sender interface {
	Send(recipient, subject, body string) error
}

// Register installs the standard library and binds mail() to sender.
func Register(rt *runner.Runtime, sender Sender) {
	stdlib.Register(rt, func(rt *runner.Runtime) {
		rt.RegisterFunc("mail", sender.Send)
	})
}

// SMTP implements the Sender interface using SMTP
type SMTP struct {
	config Config
}

// NewSMTP creates a new SMTP client
func NewSMTP(config Config) *SMTP {
	return &SMTP{config: config}
}

// Send sends an email via SMTP
func (s *SMTP) Send(recipient, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	// Create message
	message := fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s", recipient, subject, body)

	// For mailhog, no authentication is needed
	var auth smtp.Auth
	if s.config.Username != "" && s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
	}

	// Send the email
	err := smtp.SendMail(addr, auth, s.config.From, []string{recipient}, []byte(message))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
