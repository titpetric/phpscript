package info

import (
	"github.com/titpetric/phpscript/runner"
)

func init() {
	runner.RegisterBinding(Register)
}

// Register installs phpinfo().
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("phpinfo", func(_ ...any) (bool, error) {
		return true, rt.PHPInfo()
	})
}
