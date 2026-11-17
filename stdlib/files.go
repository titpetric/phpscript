package stdlib

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/ps/auth"
	"github.com/titpetric/phpscript/stdlib/ps/files"
	"github.com/titpetric/phpscript/stdlib/ps/gd"
	"github.com/titpetric/phpscript/stdlib/ps/pexec"
	"github.com/titpetric/phpscript/stdlib/ps/session"
)

// RegisterFS reroots the filesystem bindings at dir, which no path climbs out of.
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

// fsRoots pairs each area RegisterFS reroots with its register function; exec areas need Exec.
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

// selected reports whether the runtime installed area; no selection means every area.
func selected(rt *runner.Runtime, area string) bool {
	return len(runner.SelectAreas([]runner.Area{{Name: area}}, rt.Areas())) == 1
}
