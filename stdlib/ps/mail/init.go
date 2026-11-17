package mail

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the mail bindings, resolving through the configured provider or Default.
func init() {
	runner.RegisterArea(`Runtime\PS\Mail`, Register)
}
