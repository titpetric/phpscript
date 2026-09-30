package stdlib

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/files"
	"github.com/titpetric/phpscript/stdlib/gd"
	"github.com/titpetric/phpscript/stdlib/pexec"
)

// RegisterFS reroots the filesystem bindings Register installed at dir. A path
// from PHP resolves under dir and cannot climb out of it, "/" names dir itself,
// and writes are held to writable_paths; see files.RegisterRoot.
//
// stdlib/gd is rerooted with them, because a script loads an image by the path
// it would pass file_get_contents. stdlib/pexec is rerooted for a different
// reason: a command is a process rather than a path, and the root only decides
// where it starts, so that it matches what getcwd() says.
func RegisterFS(rt *runner.Runtime, dir string) {
	files.RegisterRoot(rt, dir)
	gd.RegisterRoot(rt, dir)
	pexec.RegisterRoot(rt, dir)
}
