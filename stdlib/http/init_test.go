package http_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestInit covers the wiring this package does for itself. init() contributes
// Register, the way a program wires in a database/sql driver, so a host that
// calls stdlib.Register gets the HTTP surface by having imported the package
// and names nothing. Reaching it through stdlib.Register and no direct call is
// the point: a dropped init() leaves `new HTTP\Client` as a class nothing
// declares, and every test calling Register itself would pass.
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
