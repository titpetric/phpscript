package model

import (
	"context"
)

// MailProvider resolves the named mail servers a script sends through.
//
// Nothing here returns a credential: a provider reads the host, the username
// and the password inside Send and returns the outcome alone. The interface
// lives here for the reason DatabaseProvider does, so a runtime can name it in
// its options without importing stdlib/mail, which imports the runtime.
type MailProvider interface {
	// Configured reports why a script cannot send through name, and nil when
	// it can. The Mail constructor calls it, so `new Mail` refuses a server
	// nobody configured, ahead of the first send some hours later.
	//
	// A provider fixes its servers at construction and registers none later,
	// so this verdict and the resolution Send makes for itself read the same
	// set.
	Configured(name string) error

	// Send sends one message through the named server. It reads the
	// credential here, uses it for the one delivery, and drops it.
	Send(ctx context.Context, name, recipient, subject, body string) error
}
