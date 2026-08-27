package compat

import (
	"fmt"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// registerDatetime installs the clock functions scripts time themselves with.
// These are the two that answer "how long did that take" in the epoch integers
// PHP spells them in; anything that formats, parses or does arithmetic on an
// instant is stdlib/time, where the value is a Go time.Time rather than an int.
// PHP's date/format family has no compatibility layer here on purpose, recorded
// in docs/design.md under "Dates and times".
func registerDatetime(rt *runner.Runtime) {
	// time returns the current Unix timestamp in seconds.
	rt.RegisterFunc("time", func() int64 {
		return time.Now().Unix()
	})

	// microtime(true) returns seconds as a float; the argumentless form
	// returns PHP's historical "msec sec" string.
	rt.RegisterFunc("microtime", func(asFloat ...bool) any {
		now := time.Now()
		if len(asFloat) > 0 && asFloat[0] {
			return float64(now.UnixNano()) / float64(time.Second)
		}
		sec := now.Unix()
		frac := float64(now.Nanosecond()) / float64(time.Second)
		return fmt.Sprintf("%.8f %d", frac, sec)
	})
}
