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
	// Configured reports why name cannot be delivered through, and nil when
	// it can. The Mail constructor calls it so a script naming a server
	// nobody configured is told at `new Mail`, ahead of the first
	// send some hours later.
	//
	// A provider's servers are fixed when it is built: there is no
	// registration, so this verdict and the resolution Send makes for itself
	// read the same set.
	Configured(name string) error

	// Send sends one message through the named server. The credential is
	// read here, used for the one delivery, and dropped.
	Send(ctx context.Context, name, recipient, subject, body string) error
}
