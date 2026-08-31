package stdlib

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/files"
	"github.com/titpetric/phpscript/stdlib/gd"
	"github.com/titpetric/phpscript/stdlib/pexec"
)

// RegisterFS installs filesystem IO shims rooted at dir. A path from PHP is
// treated as relative to dir and cannot climb out of it, mirroring how the
// upstream engine runs inside its project checkout. A path written from "/"
// names dir itself; there is no spelling that reaches the host outside it.
//
// Writes are held to the runtime's writable_paths when it configures any; see
// files.RegisterRoot.
//
// The shims live in stdlib/files and Register already installs them, rooted at
// the process working directory; this rebinds them to dir.
//
// The image functions in stdlib/gd read and write files too, so they are
// rebound to the same root here. A script that loads an image by the path it
// would pass to file_get_contents has to reach the same file.
//
// stdlib/pexec is rebound for a different reason: a command it starts is a
// process, not a path, and the sandbox does not reach it. What the root decides
// there is only where the command starts, so that it matches what getcwd()
// says.
func RegisterFS(rt *runner.Runtime, dir string) {
	files.RegisterRoot(rt, dir)
	gd.RegisterRoot(rt, dir)
	pexec.RegisterRoot(rt, dir)
}
