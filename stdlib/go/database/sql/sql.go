// Package sql binds go's database/sql as the Runtime\Go\Database\SQL
// namespace. A script imports it with `use Runtime\Go\Database\SQL;`, then
// opens a connection with `SQL\Open($name)`. The name must be a name the host
// registered. The host owns the DSNs, as it does for `new Database($name)`. A
// script supplies none.
package sql

import (
	"context"
	gosql "database/sql"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/ps/database"
)

// Namespace is the name a script imports.
const Namespace = modelutil.GoNamespace + `\Database\SQL`

func init() {
	runner.RegisterNamespace(Namespace, Register)
}

// Register installs the Runtime\Go\Database\SQL functions on a runtime.
func Register(rt *runner.Runtime) {
	// Runtime\Go\Database\SQL\Open opens the connection the host registered as $name.
	rt.RegisterFunc(Namespace+`\Open`, func(ctx context.Context, name string) (*database.Database, error) {
		return database.Connect(rt, ctx, name)
	})
	// Runtime\Go\Database\SQL\ErrNoRows is the error Scan reports for a query that matched no row.
	rt.SetConst(Namespace+`\ErrNoRows`, gosql.ErrNoRows)
}
