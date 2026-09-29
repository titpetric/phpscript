package bindings

import (
	"time"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the execution-limit functions to stdlib.Register.
func init() {
	runner.RegisterBinding(registerLimits)
}

// registerLimits installs the three functions a script bounds its own run with.
//
// They are one mechanism seen from three sides: the runtime holds the context
// the host gave it and a context derived from it, and these move the derivation.
// See runner/deadline.go.
func registerLimits(rt *runner.Runtime) {
	// set_time_limit bounds the rest of this script to $seconds and answers true; the clock restarts from the call, as a second call in PHP does, and 0 removes the limit. The limit is a deadline on the context the interpreter checks and every binding is handed, so it ends a Go call that is waiting as well as a PHP loop that is spinning.
	rt.RegisterFunc("set_time_limit", func(seconds int64) bool {
		rt.SetTimeLimit(time.Duration(seconds) * time.Second)
		return true
	})

	// ignore_user_abort decides whether the client closing the connection ends the script: with $enable true the script runs to its own end and asks connection_aborted() when it wants to know, and with it false, the default, the disconnect stops the script where it next looks. A time limit still applies either way.
	rt.RegisterFunc("ignore_user_abort", func(enable bool) {
		rt.SetIgnoreUserAbort(enable)
	})

	// connection_aborted returns true once the client has closed the connection, so a script can stop doing work nobody is waiting for: commit the transaction, skip rendering the page. It keeps answering after ignore_user_abort(true) detached the run from the disconnect, which is the only arrangement in which a script is still running to ask.
	rt.RegisterFunc("connection_aborted", func() bool {
		return rt.ConnectionAborted()
	})
}
