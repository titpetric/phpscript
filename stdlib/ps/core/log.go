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
	runner.RegisterArea(`Runtime\PS\Core\Log`, registerLog)
}

// registerLog installs error_log, which records the message where the host listens.
func registerLog(rt *runner.Runtime) {
	// error_log records $message where the host is listening: on the trace of
	// the request in flight, through the handler a Go host installed, and on
	// the process error stream when both are missing. It returns true. php
	// also takes $message_type, $destination and $additional_headers to
	// choose between destinations; those are not implemented.
	rt.RegisterFunc("error_log", func(message string) bool {
		rt.RecordError(errors.New(message))
		if telemetry.TraceFromContext(rt.Context()) == nil && !rt.HasErrorHandler() {
			fmt.Fprintln(os.Stderr, message)
		}
		return true
	})
}
