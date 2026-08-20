package stdlib

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/fs"
)

// RegisterFS installs filesystem IO shims rooted at dir. Reads and writes are
// confined to that directory tree, mirroring how the upstream engine runs inside
// its project checkout. Paths from PHP are treated as relative to dir.
//
// The shims live in stdlib/fs and Register already installs them, rooted at the
// process working directory; this rebinds them to dir.
func RegisterFS(rt *runner.Runtime, dir string) {
	fs.RegisterRoot(rt, dir)
}
