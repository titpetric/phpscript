package core

import (
	"context"
	"time"

	"github.com/titpetric/phpscript/runner"
)

func init() {
	runner.RegisterBinding(registerSleep)
}

// registerSleep installs the two ways a script waits.
//
// Both take the runtime context, so a wait ends when the script does: the time
// limit set_time_limit put on it, or the client going away. A sleep that ran
// past the limit it was already over would be the one place a script could not
// be stopped.
func registerSleep(rt *runner.Runtime) {
	// sleep pauses the script for $seconds and returns 0. A negative count is refused with -1, as php refuses one. The wait ends early, still returning 0, when the script runs out of time or its client goes away: the pause is on the runtime context, not on the clock alone.
	rt.RegisterFunc("sleep", func(ctx context.Context, seconds int64) int64 {
		if seconds < 0 {
			return -1
		}
		wait(ctx, time.Duration(seconds)*time.Second)
		return 0
	})

	// usleep pauses the script for $microseconds, and ends early for the same reasons sleep does. A negative count returns without waiting.
	rt.RegisterFunc("usleep", func(ctx context.Context, microseconds int64) {
		if microseconds <= 0 {
			return
		}
		wait(ctx, time.Duration(microseconds)*time.Microsecond)
	})
}

// wait sleeps for d, or until ctx ends.
func wait(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
