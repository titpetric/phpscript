package ps

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the phpscript-specific bindings (Session, SharedMemory,
// defer, shutdown functions) to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}
