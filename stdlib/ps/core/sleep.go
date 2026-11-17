package core

import (
	"context"
	"fmt"
	"time"

	"github.com/titpetric/phpscript/runner"
)

func init() {
	runner.RegisterArea(`Runtime\PS\Core\Sleep`, registerSleep)
}

// registerSleep installs sleep and usleep, ending early when the script or client stops.
func registerSleep(rt *runner.Runtime) {
	// sleep pauses the script for $seconds and returns 0. A negative count
	// throws, as it does in php. The wait ends early, still returning 0, when
	// the script runs out of time or the client it answers goes away.
	rt.RegisterFunc("sleep", func(seconds int64) (int64, error) {
		if seconds < 0 {
			return 0, fmt.Errorf("sleep(): Argument #1 ($seconds) must be greater than or equal to 0")
		}
		wait(rt.ClientContext(), rt.Context(), time.Duration(seconds)*time.Second)
		return 0, nil
	})

	// usleep pauses the script for $microseconds, and ends early for the same
	// reasons sleep does. A negative count throws, as it does in php.
	rt.RegisterFunc("usleep", func(microseconds int64) error {
		if microseconds < 0 {
			return fmt.Errorf("usleep(): Argument #1 ($microseconds) must be greater than or equal to 0")
		}
		if microseconds == 0 {
			return nil
		}
		wait(rt.ClientContext(), rt.Context(), time.Duration(microseconds)*time.Microsecond)
		return nil
	})
}

// wait sleeps for d or until the client or script context ends.
func wait(client, script context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()

	var clientDone, scriptDone <-chan struct{}
	if client != nil {
		clientDone = client.Done()
	}
	if script != nil {
		scriptDone = script.Done()
	}
	select {
	case <-timer.C:
	case <-clientDone:
	case <-scriptDone:
	}
}
