package mail

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the mail bindings to stdlib.Register: `new Mail` and mail()
// are available to every runtime. Both resolve through the runtime's configured
// mail provider, and through Default when a host configured none and they exist
// with or without a host mail block and refuse catchably without one.
func init() {
	runner.RegisterBinding(Register)
}
