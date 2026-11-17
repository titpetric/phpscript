package crypto

import (
	"github.com/titpetric/phpscript/runner"
)

// init registers the password, CSPRNG, digest and identifier areas.
func init() {
	runner.RegisterArea(`Runtime\PS\Crypto`, Register)
	runner.RegisterArea(`Runtime\PS\Crypto\Random`, RegisterRandom)
	runner.RegisterArea(`Runtime\PS\Crypto\Hash`, RegisterHash)
	runner.RegisterArea(`Runtime\PS\Crypto\Identifiers`, RegisterIdentifiers)
}
