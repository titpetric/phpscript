package database

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the Database and Database\Migrate bindings to
// stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}
