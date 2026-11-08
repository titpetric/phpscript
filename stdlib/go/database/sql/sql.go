// Package sql binds go's database/sql as the Runtime\Go\Database\SQL
// namespace. A script imports it with `use Runtime\Go\Database\SQL;` and opens
// a connection with `SQL\Open($name)`. The name is one the host registered:
// the host owns the DSNs, as it does for `new Database($name)`, and a script
// supplies none.
package sql

import (
	"context"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/ps/database"
)

// Namespace is the name a script imports.
const Namespace = modelutil.GoNamespace + `\Database\SQL`

func init() {
	runner.RegisterNamespace(Namespace, Register)
}

// Register installs the Runtime\Go\Database\SQL functions on a runtime that imported them.
func Register(rt *runner.Runtime) {
	// Runtime\Go\Database\SQL\Open opens the connection the host registered as $name.
	rt.RegisterFunc(Namespace+`\Open`, func(ctx context.Context, name string) (*database.Database, error) {
		return database.Connect(rt, ctx, name)
	})
}
