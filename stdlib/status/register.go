package status

import (
	"github.com/titpetric/phpscript/runner"
)

// Register installs symbols into the runtime.
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("span", Span)
}
