package crypto

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the password hashing and CSPRNG bindings to
// stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
	runner.RegisterBinding(RegisterRandom)
}
