package auth

import (
	"context"
)

// Storage is a user database for authentication. It answers the question a
// login asks, and whether a database exists to ask it of.
//
// This is the Auth\Storage\* contract, the arrangement stdlib/ps/session's
// Storage uses one level up: a named interface with a driver registered in the
// namespace. A host with a user table implements these two methods, and
// Session\Manager logs in with that table unchanged.
type Storage interface {
	// Authenticate reports the user key names when secret is theirs. A nil
	// user refuses and raises no error: a wrong password, an unknown key or a
	// malformed key all answer the same way. Neither a caller nor the
	// caller's timing tells them apart.
	//
	// An error names a store nothing could read: a misconfiguration, where a
	// refusal is a failed login.
	Authenticate(ctx context.Context, key, secret string) (User, error)

	// Exists reports whether a user database exists: the store is present and
	// holds at least one account.
	//
	// A manager takes a default store on this answer and the default fails no
	// tree that has no user database: construction resolves where the store
	// would sit. Exists reports whether any account sits there. A login page
	// prints that state, and an installer asks it first.
	Exists() (bool, error)
}
