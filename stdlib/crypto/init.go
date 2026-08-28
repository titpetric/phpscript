package crypto

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the password hashing, CSPRNG, message-digest and
// id-generator bindings to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
	runner.RegisterBinding(RegisterRandom)
	runner.RegisterBinding(RegisterHash)
	runner.RegisterBinding(RegisterIdentifiers)
}
