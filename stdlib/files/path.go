package files

import (
	stdpath "path"
	"path/filepath"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerPaths installs the string-only path functions. They answer about the
// shape of a path and never touch the filesystem, so they are not bound to the
// root and work the same on a path that does not exist.
func registerPaths(rt *runner.Runtime) {
	// dirname returns the parent directory of $path; the $levels argument is not accepted.
	rt.RegisterFunc("dirname", func(path string) string {
		return stdpath.Dir(strings.TrimRight(filepath.ToSlash(path), "/"))
	})
	// basename returns the trailing name component of $path; the $suffix argument is not accepted.
	rt.RegisterFunc("basename", func(path string) string {
		return stdpath.Base(filepath.ToSlash(path))
	})
}
