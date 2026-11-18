package core

import (
	"context"
	"testing"
	"time"
)

// TestWait covers the wait that sleep() and usleep() share, plus the ways it
// ends early.
//
// A sleep that ran past a cancelled context would leave one place nothing
// could stop a script. Both contexts end the wait. The client's ends when the
// client goes away; the script's own ends when it runs out of time. A duration
// of zero or less returns without arming a timer.
func TestWait(t *testing.T) {
	t.Parallel()
	const forever = 30 * time.Second

	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}

	for _, tc := range []struct {
		name           string
		client, script context.Context
		d              time.Duration
	}{
		{name: "zero duration", client: context.Background(), script: context.Background()},
		{name: "negative duration", client: context.Background(), script: context.Background(), d: -time.Second},
		{name: "the client went away", client: cancelled(), script: context.Background(), d: forever},
		{name: "the script ran out of time", client: context.Background(), script: cancelled(), d: forever},
		{name: "no contexts at all", d: time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			wait(tc.client, tc.script, tc.d)
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Errorf("the wait took %v, want a prompt return", elapsed)
			}
		})
	}
}

// TestWaitRunsTheClockDown covers a wait neither context cancels, lasting its full duration.
func TestWaitRunsTheClockDown(t *testing.T) {
	t.Parallel()
	const d = 20 * time.Millisecond

	start := time.Now()
	wait(context.Background(), context.Background(), d)

	if elapsed := time.Since(start); elapsed < d {
		t.Errorf("the wait returned after %v, want at least %v", elapsed, d)
	}
}
