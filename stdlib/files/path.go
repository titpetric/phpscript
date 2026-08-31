package files

import (
	stdpath "path"
	"path/filepath"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerWorkDir installs the working-directory pair. Unlike the functions
// below they are bound to the root, because the directory they move is a
// position inside it.
//
// The directory is per-runtime state, and a host builds one runtime per
// request, so a script that moves it moves nothing another request can see.
// os.Chdir is never called: the process working directory is shared by every
// request in flight and by the host itself, and there is no point at which
// owning it would be correct.
func registerWorkDir(rt *runner.Runtime) {
	// chdir changes the working directory relative paths resolve against and returns whether it could; the directory is this request's own, and a path that would climb out of the source filesystem's root stops at it.
	rt.RegisterFunc("chdir", rt.SetWorkDir)
	// getcwd returns the working directory, written from the source filesystem's root: "/" for the root itself, "/app" for a directory below it. PHP answers a host path; a runtime whose scripts may be served out of an embedded tree has none to answer with.
	rt.RegisterFunc("getcwd", rt.WorkDirPath)
}

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
