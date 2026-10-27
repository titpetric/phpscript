// Package session implements Session\Manager and the Session\Storage\*
// family: an HTTP-only cookie carrying an opaque, randomly generated session
// ID, and the storage the data behind that ID lives in. Memory storage lasts
// as long as the process; disk storage is a file per session under a directory
// the host chooses, and prunes by modification time.
//
// A manager wraps whichever storage it receives for tracing and a request's
// trace shows which backend it paid for and whether the session exists. The
// trace omits the session ID: that ID is the credential in the cookie.
//
// Who a session belongs to is stdlib/auth's subject. A manager holds the user
// database a host gave it and puts one question to that, in login();
// everything else here is the cookie and the storage.
//
// Importing the package wires it in; stdlib/imports.go does that.
package session

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/auth"
	"github.com/titpetric/phpscript/stdlib/logger"
)

// init contributes the session bindings to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs the session classes at the working directory.
func Register(rt *runner.Runtime) {
	RegisterRoot(rt, ".")
}

// RegisterRoot installs the session classes with disk storage rooted at dir. A
// $storage_path the script names resolves against dir, cannot climb out of it,
// and must sit inside the runtime's writable_paths and the directory it creates
// is one the script could also have written a file into.
//
// Without that, the constructor mkdir'd whatever the script named: a vhost could
// create a directory anywhere the process could write, and read and write session
// files there, while file_exists() on the same path answered false.
func RegisterRoot(rt *runner.Runtime, dir string) {
	// Session\Storage\Memory is session storage backed by process memory;
	// sessions vanish when the process exits.
	rt.RegisterConstructor("Session\\Storage\\Memory", NewStorageMemory)
	// Session\Storage\Disk is session storage backed by files under
	// $storage_path, which resolves inside the application root and must be
	// writable; with no path it uses a directory of this application's own
	// under the operating system's temporary directory.
	rt.RegisterConstructor("Session\\Storage\\Disk", func(storagePaths ...string) (*StorageDisk, error) {
		return newRootedStorageDisk(rt, dir, storagePaths...)
	})
	// Session\Manager starts, reads and validates the request's session
	// against the given $storage, and logs a user in against the given
	// $users. With no arguments it takes disk storage and the application's
	// own .htpasswd and `new Session\Manager` is a working login.
	//
	// The default store opens the file and defers the parse to the login
	// that reads it: a manager constructs on every request, and a page
	// that never calls login() does not pay for the account list.
	rt.RegisterConstructor("Session\\Manager", func(storage Storage, users auth.Storage) (*Manager, error) {
		if storage == nil {
			storage = defaultStorage(rt, dir)
		}
		if users == nil {
			store, err := auth.DefaultPasswordFileRoot(rt, dir)
			if err != nil {
				return nil, err
			}
			users = store
		}
		return NewManager(storage, users)
	})
}

// defaultStorage is the storage a manager takes when the script names none:
// disk under the application root, and memory when disk cannot be had.
//
// The fallback writes a log line, because memory storage is a different
// promise. It lives as long as the runtime that built it, and a server forks
// one per worker and a session minted on one worker is invisible to the next
// and a login fails at random. One line costs less to read than that symptom.
// The line names the path and the reason, and the trace names the backend on
// every session span.
//
// Warn and not Error: the request that constructed this serves fine, and
// failing its trace would report an outage over a misconfiguration.
func defaultStorage(rt *runner.Runtime, dir string) Storage {
	disk, err := newRootedStorageDisk(rt, dir)
	if err == nil {
		return disk
	}

	logger.New(rt.Context(), "session").Warn(
		"disk storage is unavailable, falling back to memory; sessions last as long as the worker and are not shared between them",
		"path", DefaultStoragePath(dir),
		"error", err,
	)
	return NewStorageMemory()
}
