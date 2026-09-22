package mail

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the mail bindings to stdlib.Register: `new Mail` and mail()
// are available to every runtime. Both resolve through the runtime's configured
// mail provider, and through Default when a host configured none, so they exist
// whether or not the host has a mail block and refuse catchably when it does
// not.
func init() {
	runner.RegisterBinding(Register)
}
