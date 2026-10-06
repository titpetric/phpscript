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

// Register installs the session storage and manager classes, rooted at the
// process working directory the way the filesystem shims are.
func Register(rt *runner.Runtime) {
	RegisterRoot(rt, ".")
}

// RegisterRoot installs the session classes with disk storage rooted at dir. A
// $storage_path the script names resolves against dir and cannot climb out of
// it, and is held to the runtime's writable_paths, so the directory it creates is
// one the script could also have written a file into.
//
// Without that, the constructor mkdir'd whatever the script named: a vhost could
// create a directory anywhere the process could write, and read and write session
// files there, while file_exists() on the same path answered false.
func RegisterRoot(rt *runner.Runtime, dir string) {
	// Session\Storage\Memory is session storage backed by process memory; sessions vanish when the process exits.
	rt.RegisterConstructor("Session\\Storage\\Memory", NewStorageMemory)
	// Session\Storage\Disk is session storage backed by files under $storage_path, which resolves inside the application root and must be writable; with no path it uses a directory of this application's own under the operating system's temporary directory.
	rt.RegisterConstructor("Session\\Storage\\Disk", func(storagePaths ...string) (*StorageDisk, error) {
		return newRootedStorageDisk(rt, dir, storagePaths...)
	})
	// Session\Manager starts, reads and validates the request's session against the given $storage.
	rt.RegisterConstructor("Session\\Manager", NewManager)
}
