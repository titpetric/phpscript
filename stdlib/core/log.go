package core

import (
	"errors"
	"fmt"
	"os"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

// init contributes the error log to stdlib.Register.
func init() {
	runner.RegisterBinding(registerLog)
}

// registerLog installs the one way a script says something went wrong without
// throwing.
//
// Where it goes depends on what is listening, as php's error_log
// does too: it writes to the SAPI's log, and the SAPI decides what that is.
// Here a request being traced puts it on the trace, a host that installed an
// error handler gets it there, and a run with neither - a command line script -
// gets the process error stream, because an error log that discards is worse
// than no error log.
func registerLog(rt *runner.Runtime) {
	// error_log records $message where the host is listening: on the trace of the request being served, through the handler a Go host installed, and on the process error stream when neither is there. It returns true. php also takes $message_type, $destination and $additional_headers to choose between destinations; those are not implemented.
	rt.RegisterFunc("error_log", func(message string) bool {
		rt.RecordError(errors.New(message))
		if telemetry.TraceFromContext(rt.Context()) == nil && !rt.HasErrorHandler() {
			fmt.Fprintln(os.Stderr, message)
		}
		return true
	})
}
