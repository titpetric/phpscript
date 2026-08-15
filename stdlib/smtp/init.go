package smtp

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the SMTP class binding to stdlib.Register, making `new SMTP`
// available to scripts.
func init() {
	runner.RegisterBinding(RegisterSMTP)
}
