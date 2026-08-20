package compat

import (
	"fmt"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// registerDatetime installs the clock functions scripts time themselves with.
// PHP's date/format family is not implemented; these are the two functions a
// script needs to measure its own execution.
func registerDatetime(rt *runner.Runtime) {
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
