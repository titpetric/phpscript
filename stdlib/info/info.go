package info

import (
	"github.com/titpetric/phpscript/runner"
)

func init() {
	runner.RegisterBinding(Register)
}

// Register installs phpinfo().
func Register(rt *runner.Runtime) {
	// phpinfo prints a compact runtime report, the same one the phpscript info command shows.
	rt.RegisterFunc("phpinfo", func(_ ...any) (bool, error) {
		return true, rt.PHPInfo()
	})
}
