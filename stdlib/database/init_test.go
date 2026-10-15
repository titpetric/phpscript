package database_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestDatabaseBindingsAreContributed covers the wiring the package init does.
// A binding package registers its installer through runner.RegisterBinding, and
// stdlib.Register runs every contributed installer, so importing the package
// is the whole of what a host does to put the Database class within reach of a
// script. A host that constructs a Runtime without stdlib has the installer
// list and picks from it.
func TestDatabaseBindingsAreContributed(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)

	for _, name := range []string{"Database", "Database\\Migrate"} {
		if _, ok := rt.LookupConstructor(name); !ok {
			t.Errorf("new %s resolves to nothing on a runtime stdlib.Register set up", name)
		}
	}
	for _, name := range []string{"Database::connections", "Database::register"} {
		if _, ok := rt.LookupFunc(name); !ok {
			t.Errorf("%s() resolves to nothing on a runtime stdlib.Register set up", name)
		}
	}
}
