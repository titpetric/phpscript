package stdlib

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/ps/auth"
	"github.com/titpetric/phpscript/stdlib/ps/files"
	"github.com/titpetric/phpscript/stdlib/ps/gd"
	"github.com/titpetric/phpscript/stdlib/ps/pexec"
	"github.com/titpetric/phpscript/stdlib/ps/session"
)

// RegisterFS reroots the filesystem bindings Register installed at dir, which
// a path from PHP resolves under and cannot climb out of. gd, auth and session
// go with them, and pexec where the mount included it: the root is where a
// command starts, so that getcwd() agrees with it.
func RegisterFS(rt *runner.Runtime, dir string) {
	for _, fs := range fsRoots {
		if fs.exec && rt.Profile()&Exec == 0 {
			continue
		}
		if selected(rt, fs.area) {
			fs.register(rt, dir)
		}
	}
}

// fsRoots pairs each area RegisterFS reroots with the function that does it,
// under the name its package registers; exec ones need the Exec profile.
var fsRoots = []struct {
	area     string
	exec     bool
	register func(*runner.Runtime, string)
}{
	{`Runtime\PS\Files`, false, files.RegisterRoot},
	{`Runtime\PS\GD`, false, gd.RegisterRoot},
	{`Runtime\PS\Auth`, false, auth.RegisterRoot},
	{`Runtime\PS\Session`, false, session.RegisterRoot},
	{`Runtime\PS\Exec`, true, pexec.RegisterRoot},
}

// selected reports whether the runtime's areas include the named one: a runtime
// built with no selection carries every area, and one built with a selection
// reroots only the bindings of the areas it installed.
func selected(rt *runner.Runtime, area string) bool {
	return len(runner.SelectAreas([]runner.Area{{Name: area}}, rt.Areas())) == 1
}
