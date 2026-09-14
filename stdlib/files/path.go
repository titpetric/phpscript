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
	// dirname returns the parent directory of $path, walking up $levels parents when given.
	rt.RegisterFunc("dirname", func(path string, levels ...int64) string {
		up := int64(1)
		if len(levels) > 0 && levels[0] > 1 {
			up = levels[0]
		}
		p := strings.TrimRight(filepath.ToSlash(path), "/")
		for ; up > 0; up-- {
			p = stdpath.Dir(p)
		}
		return p
	})
	// basename returns the trailing name component of $path, less $suffix when the name ends with it. The empty and root paths answer "", as PHP's do.
	rt.RegisterFunc("basename", func(path string, suffix ...string) string {
		p := strings.TrimRight(filepath.ToSlash(path), "/")
		if p == "" {
			return ""
		}
		base := stdpath.Base(p)
		if len(suffix) > 0 && suffix[0] != "" && base != suffix[0] && strings.HasSuffix(base, suffix[0]) {
			base = strings.TrimSuffix(base, suffix[0])
		}
		return base
	})
}
