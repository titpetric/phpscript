package http_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestInit covers the wiring this package performs itself. init() contributes
// Register, the way a program wires in a database/sql driver; a host that
// calls stdlib.Register gets the HTTP surface by having imported the package,
// naming nothing. A test must reach it via stdlib.Register: a dropped init()
// leaves `new HTTP\Client` as a class nothing
// declares. A suite calling Register itself would pass.
func TestInit(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)

	for _, class := range []string{"HTTP\\Request", "HTTP\\Client", "HTTP\\Mux"} {
		declared, err := rt.ClassExists(class, false)
		if err != nil {
			t.Fatalf("ClassExists(%s): %v", class, err)
		}
		if !declared {
			t.Errorf("%s is not declared, so init() did not contribute Register", class)
		}
	}
}
