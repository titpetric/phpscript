package bindings

import (
	"math"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// limitFor turns a count of seconds into a duration without wrapping. The
// multiply overflows around 292 years of nanoseconds, which would turn an
// absurd limit into a short one rather than into no limit at all.
func limitFor(seconds int64) time.Duration {
	const maxSeconds = int64(math.MaxInt64 / int64(time.Second))
	if seconds > maxSeconds {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(seconds) * time.Second
}

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
		rt.SetTimeLimit(limitFor(seconds))
		return true
	})

	// ignore_user_abort decides whether the client closing the connection ends the script: with $enable true the script runs to its own end and asks connection_aborted() when it wants to know, and with it false, the default, the disconnect stops the script where it next looks. A time limit still applies either way.
	rt.RegisterFunc("ignore_user_abort", func(enable ...bool) {
		// Variadic so that the read-only spelling php has, ignore_user_abort()
		// with no argument, leaves the setting alone. A plain bool parameter is
		// zero-padded when the argument is missing, so `if
		// (ignore_user_abort())` would have turned off the thing it was asking
		// about. It still answers nothing; docs/README.md records that.
		if len(enable) == 0 {
			return
		}
		rt.SetIgnoreUserAbort(enable[0])
	})

	// connection_aborted returns true once the client has closed the connection, so a script can stop doing work nobody is waiting for: commit the transaction, skip rendering the page. It keeps answering after ignore_user_abort(true) detached the run from the disconnect, which is the only arrangement in which a script is still running to ask.
	rt.RegisterFunc("connection_aborted", func() bool {
		return rt.ConnectionAborted()
	})
}
