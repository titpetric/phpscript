package fs

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerPaths installs the string-only path functions. They answer about the
// shape of a path and never touch the filesystem, so they are not bound to the
// root and work the same on a path that does not exist.
func registerPaths(rt *runner.Runtime) {
	rt.RegisterFunc("dirname", func(p string) string {
		return path.Dir(strings.TrimRight(filepath.ToSlash(p), "/"))
	})
	rt.RegisterFunc("basename", func(p string) string {
		return path.Base(filepath.ToSlash(p))
	})
}
