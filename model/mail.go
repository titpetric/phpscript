package model

import (
	"context"
)

// MailProvider resolves the named mail servers a script delivers through.
//
// Nothing here returns a credential. A provider holds the host, the username
// and the password of every server it was configured with, reads them inside
// Send, and hands back only whether the delivery succeeded. A binding, and
// through it a script, names a server; it never holds what the server is
// reached with, and there is no call that lists what was configured.
//
// The interface lives here for the same reason DatabaseProvider does: a
// runtime names it in its options without depending on the package that
// implements it, which is stdlib/mail and which imports the runtime to
// register its bindings. A runtime knows only that something answers to a
// server name; which servers exist is the host's business, and a virtual host
// answers differently from the process it shares.
type MailProvider interface {
	// Configured reports why name cannot be delivered through, and nil when
	// it can. The Mail constructor calls it so a script naming a server
	// nobody configured is told at `new Mail`, rather than at the first
	// send some hours later.
	//
	// A provider's servers are fixed when it is built - there is no
	// registration, by design - so this answer and the resolution Send makes
	// for itself cannot disagree.
	Configured(name string) error

	// Send delivers one message through the named server. The credential is
	// read here, used for the one delivery, and dropped.
	Send(ctx context.Context, name, recipient, subject, body string) error
}
