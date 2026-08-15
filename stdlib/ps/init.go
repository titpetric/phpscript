package ps

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the host-backed platform bindings (Database,
// Database\Migrate, Session, SharedMemory, defer, shutdown functions) to
// stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}
