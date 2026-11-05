package crypto

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the password hashing, CSPRNG, message-digest and
// id-generator bindings to stdlib.Register.
func init() {
	runner.RegisterArea(`Runtime\PS\Crypto`, Register)
	runner.RegisterArea(`Runtime\PS\Crypto\Random`, RegisterRandom)
	runner.RegisterArea(`Runtime\PS\Crypto\Hash`, RegisterHash)
	runner.RegisterArea(`Runtime\PS\Crypto\Identifiers`, RegisterIdentifiers)
}
