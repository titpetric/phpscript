// Package fs provides the filesystem shims phpscript exposes to PHP: path
// helpers, reads, writes, streams, and the uploaded-file functions that go with
// $_FILES. They are grouped here rather than in stdlib because they are the one
// part of the standard library bound to a directory on the host, and because a
// host that runs untrusted scripts may want to leave them out.
//
// Go's fs.FS is read-only, so writes (fopen/fwrite/mkdir) use the os package
// against the same root; the runner's include resolution still uses the fs.FS
// abstraction passed to runner.New.
package fs

import (
	"path"
	"path/filepath"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the filesystem bindings to stdlib.Register, rooted at the
// process working directory, which is where a CLI run reads and writes anyway.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs the filesystem shims rooted at the process working
// directory. A host serving scripts out of a project directory binds that
// instead, with RegisterRoot.
func Register(rt *runner.Runtime) {
	RegisterRoot(rt, ".")
}

// RegisterRoot installs the filesystem shims rooted at dir. Reads and writes
// are confined to that directory tree, mirroring how the upstream engine runs
// inside its project checkout. Paths from PHP are treated as relative to dir.
//
// It replaces whatever root was installed before it, so a host calls it after
// stdlib.Register (stdlib.RegisterFS does exactly that).
func RegisterRoot(rt *runner.Runtime, dir string) {
	r := root{rt: rt, dir: dir}

	registerPaths(rt)
	registerReads(rt, r)
	registerWrites(rt, r)
	registerStreams(rt, r)
	registerUploads(rt, r)
}

// root is the directory the shims are bound to, and the runtime they resolve
// reads through. Every binding that takes a path from PHP goes through it, so
// the mapping from script path to host path is stated once.
type root struct {
	rt  *runner.Runtime
	dir string
}

// resolve maps a path a script supplied onto the host filesystem. A relative
// path is cleaned against the root, so it cannot climb out of it; an absolute
// path is the script's own business and is only cleaned.
func (r root) resolve(p string) string {
	if filepath.IsAbs(p) {
		return path.Clean(p)
	}
	clean := path.Clean("/" + filepath.ToSlash(p))
	return filepath.Join(r.dir, filepath.FromSlash(clean))
}
