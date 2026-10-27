package mail_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestMailBindingsAreContributed covers the wiring the package init does. A
// binding package registers its installer through runner.RegisterBinding, and
// stdlib.Register runs every contributed installer and importing the package
// is the whole of what a host does to put mail() and the Mail class within
// reach of a script. Both resolve through the runtime's provider, and through
// the package default when a host configured none and a runtime set up with no
// mail block still answers function_exists("mail").
func TestMailBindingsAreContributed(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{})
	stdlib.Register(rt)

	if _, ok := rt.LookupConstructor("Mail"); !ok {
		t.Error("new Mail resolves to nothing on a runtime stdlib.Register set up")
	}
	if !rt.FunctionExists("mail") {
		t.Error("function_exists(\"mail\") is false on a runtime stdlib.Register set up")
	}
}
