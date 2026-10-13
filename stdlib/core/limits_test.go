package core

import (
	"math"
	"testing"
	"time"
)

// TestLimitFor covers turning set_time_limit's count of seconds into a
// duration without wrapping. The multiply overflows past roughly 292 years of
// nanoseconds, so a count above that saturates: an absurd limit stays a long
// one, where a wrapped product would cut the script off early.
func TestLimitFor(t *testing.T) {
	const maxSeconds = int64(math.MaxInt64 / int64(time.Second))

	for _, tc := range []struct {
		seconds int64
		want    time.Duration
	}{
		{seconds: 0, want: 0},
		{seconds: 1, want: time.Second},
		{seconds: 30, want: 30 * time.Second},
		{seconds: maxSeconds, want: time.Duration(maxSeconds) * time.Second},
		{seconds: maxSeconds + 1, want: time.Duration(math.MaxInt64)},
		{seconds: math.MaxInt64, want: time.Duration(math.MaxInt64)},
	} {
		got := limitFor(tc.seconds)
		if got != tc.want {
			t.Errorf("limitFor(%d) = %v, want %v", tc.seconds, got, tc.want)
		}
		if got < 0 {
			t.Errorf("limitFor(%d) wrapped to %v", tc.seconds, got)
		}
	}
}
