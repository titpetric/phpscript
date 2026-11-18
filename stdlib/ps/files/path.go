package files

import (
	stdpath "path"
	"path/filepath"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerWorkDir installs chdir and getcwd over per-runtime state rooted in the filesystem.
func registerWorkDir(rt *runner.Runtime) {
	// chdir changes the working directory for relative resolution and returns
	// whether it could. The directory belongs to this request; a path that
	// would climb out of the source filesystem's root stops at it.
	rt.RegisterFunc("chdir", rt.SetWorkDir)
	// getcwd returns the working directory, written from the source
	// filesystem's root: "/" for the root itself, "/app" for a child
	// directory. PHP answers a host path; a runtime that may serve its scripts
	// out of an embedded tree has no host path to answer with.
	rt.RegisterFunc("getcwd", rt.WorkDirPath)
}

// registerPaths installs path functions that read a path's shape without the filesystem.
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
	// basename returns the trailing name component of $path, less $suffix
	// when the name ends in that suffix. The empty and root paths answer "", as
	// PHP's do.
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
