// Package session implements Session\Manager and the Session\Storage\*
// family: an HTTP-only cookie carrying an opaque, randomly generated session
// ID, and the storage the data behind that ID lives in. Memory storage lasts
// as long as the process; disk storage is a file per session under a directory
// the host chooses, and prunes by modification time.
//
// A manager wraps whichever storage it is given for tracing, so a request's
// trace shows which backend it paid for and whether the session was there. The
// session ID is never recorded: it is the credential in the cookie.
//
// It is wired in by importing it; stdlib/imports.go does that.
package session

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the session bindings to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs the session storage and manager classes.
func Register(rt *runner.Runtime) {
	// Session\Storage\Memory is session storage backed by process memory; sessions vanish when the process exits.
	rt.RegisterConstructor("Session\\Storage\\Memory", NewStorageMemory)
	// Session\Storage\Disk is session storage backed by files under $storage_path; with no path it uses the operating system's temporary directory.
	rt.RegisterConstructor("Session\\Storage\\Disk", NewStorageDisk)
	// Session\Manager starts, reads and validates the request's session against the given $storage.
	rt.RegisterConstructor("Session\\Manager", NewManager)
}
