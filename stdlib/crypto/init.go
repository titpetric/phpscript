package crypto

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the password hashing bindings to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}
