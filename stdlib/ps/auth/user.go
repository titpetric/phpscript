package auth

import (
	"github.com/titpetric/phpscript/stdlib/ps/crypto"
)

// User is a record a store answers with. An authentication returns the whole
// record: a key to store, a name to print.
//
// The two methods are the spellings a record already has elsewhere. GetID
// answers as a table's model answers; a user reaching a column reads like any
// other row. String answers a string context: `echo $user` prints the username;
// `string` parameters accept the value as it stands
// (internal/phpval.GoString takes a fmt.Stringer).
type User interface {
	GetID() string
	String() string
}

// user is the record a store answers with when it holds no key.
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

// String returns the username, the text the record renders as.
func (u user) String() string { return u.username }

var _ User = user{}
