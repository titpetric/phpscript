package auth

import (
	"github.com/titpetric/phpscript/stdlib/ps/crypto"
)

// User is one record a store answers with, and the whole of what an
// authentication returns: a key to store, and a name to print.
//
// The two methods are the spellings a record already has elsewhere. GetID is
// what a table's model answers and a user reaching a column reads like any
// other row; String is what a string context calls and `echo $user` prints the
// username and a `string` parameter takes it without a cast
// (internal/phpval.GoString takes a fmt.Stringer).
type User interface {
	GetID() string
	String() string
}

// user is the record a store with no stored key answers with.
type user struct {
	username string
	id       string
}

// newUser computes the record for a username.
func newUser(username string) user {
	return user{username: username, id: crypto.ULIDFrom(username)}
}

// GetID is the primary key, 26 characters of Crockford base32.
func (u user) GetID() string { return u.id }

// String returns the username, the text the record renders as in a string context.
func (u user) String() string { return u.username }

var _ User = user{}
