package ps

import "github.com/titpetric/phpscript/runner"

// Register installs the phpscript extensions into the runtime.
func Register(rt *runner.Runtime) {
	RegisterDatabase(rt)
}
