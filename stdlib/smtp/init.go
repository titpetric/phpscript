package smtp

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the SMTP class binding and the mail() fallback to
// stdlib.Register: `new SMTP` is available to scripts, and mail() exists
// everywhere, refusing catchably until a host binds a configured sender over
// it (RegisterConfig).
func init() {
	runner.RegisterBinding(RegisterSMTP)
	runner.RegisterBinding(registerUnconfiguredMail)
}

func registerUnconfiguredMail(rt *runner.Runtime) {
	Register(rt, Unconfigured())
}
