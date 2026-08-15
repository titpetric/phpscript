package status

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the request tracing bindings (start_span) to
// stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}
