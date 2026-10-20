// Package auth implements Auth\Storage\*: the user database a login puts its
// credential to, and the record one answers with.
//
// A site names its user database and never reads it, the rule
// docs/security.md states for a connection and a mail server. The only store
// here is an apache-format password file: a script names the file, this package
// reads the hashes and verifies against them, and no value holding a hash
// reaches PHP. A script gets a verdict, a primary key and a username.
//
// A record derives its key from its username, so a user database with no id
// column still has stable primary keys for other tables to carry.
// crypto.ULIDFrom is the derivation and stdlib/auth/user.go is the record.
//
// Session\Manager turns a verdict into a session; this package knows nothing
// about cookies. Importing it wires it in, and stdlib/imports.go does that.
package auth

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the auth bindings to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs the user stores, rooted at the process working directory
// the way the filesystem shims are.
func Register(rt *runner.Runtime) {
	RegisterRoot(rt, ".")
}

// RegisterRoot installs the user stores with paths resolved against dir, so a
// file a script names is the file it would read with file_get_contents and no
// spelling reaches outside the application root.
func RegisterRoot(rt *runner.Runtime, dir string) {
	// Auth\Storage\PasswordFile authenticates against an apache-format
	// password file of `username:hash` lines, $path inside the application
	// root; with no path it reads .htpasswd there. Lines may be bcrypt,
	// argon2, apache md5 or {SHA}; the constructor refuses anything else and
	// names the line.
	rt.RegisterConstructor("Auth\\Storage\\PasswordFile", func(paths ...string) (*PasswordFile, error) {
		return NewPasswordFileRoot(rt, dir, paths...)
	})
}
