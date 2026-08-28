package stdlib

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/files"
	"github.com/titpetric/phpscript/stdlib/gd"
)

// RegisterFS installs filesystem IO shims rooted at dir. A path from PHP is
// treated as relative to dir and cannot climb out of it, mirroring how the
// upstream engine runs inside its project checkout. An absolute path is taken
// as the script wrote it.
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
func RegisterFS(rt *runner.Runtime, dir string) {
	files.RegisterRoot(rt, dir)
	gd.RegisterRoot(rt, dir)
}
