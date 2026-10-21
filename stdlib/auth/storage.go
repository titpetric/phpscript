package auth

import (
	"context"
)

// Storage is a user database to authenticate against: the one question a login
// asks, and whether there is a database to ask it of.
//
// This is the Auth\Storage\* contract, the arrangement stdlib/session's
// Storage has one level up - a named interface, and a driver registered under
// the namespace. A host with its own user table implements these two methods
// and Session\Manager logs in against it unchanged.
type Storage interface {
	// Authenticate reports the user key names when secret is theirs. A nil
	// user refuses, with no error: a wrong password, an unknown key and a
	// malformed key all answer the same way, so neither a caller nor a
	// caller's timing tells them apart.
	//
	// An error names a store nothing could read: a misconfiguration, where a
	// refusal is a failed login.
	Authenticate(ctx context.Context, key, secret string) (User, error)

	// Exists reports whether there is a user database to ask: the store is
	// there, and it holds at least one account.
	//
	// A manager takes a default store on this answer, so the default fails no
	// tree that has no user database: construction resolves where the store
	// would sit, and this says whether any account sits there. A login page
	// prints that state, and an installer asks it first.
	Exists() (bool, error)
}
