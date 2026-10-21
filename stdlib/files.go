package stdlib

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/files"
	"github.com/titpetric/phpscript/stdlib/gd"
	"github.com/titpetric/phpscript/stdlib/pexec"
	"github.com/titpetric/phpscript/stdlib/session"
)

// RegisterFS reroots the filesystem bindings Register installed at dir, which
// a path from PHP resolves under and cannot climb out of. gd and session go
// with them, and pexec where the mount included it: the root is where a
// command starts, so that getcwd() agrees with it.
func RegisterFS(rt *runner.Runtime, dir string) {
	files.RegisterRoot(rt, dir)
	gd.RegisterRoot(rt, dir)
	session.RegisterRoot(rt, dir)
	if rt.Profile()&Exec != 0 {
		pexec.RegisterRoot(rt, dir)
	}
}
