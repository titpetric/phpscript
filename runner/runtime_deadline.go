package runner

import (
	"context"
	"time"
)

// This file is the execution deadline and the client connection: two things a
// script ends for, kept apart because they end it differently.
//
// bindContext builds the context a binding takes once per session, and nothing
// rebuilds it. A time limit is a timer that cancels that context, and changing
// the limit resets the timer and derives no new context, because a binding that
// blocks - HTTP\Server::wait, which is a serving script's whole run - parks on
// the context it holds and a rebuild would end it. A handler calling
// set_time_limit is the ordinary way to reach that.
//
// The per-statement check reads the connection beside the context and merges
// neither into the other, for the same reason: a request is one of many and the
// context belongs to the script.
//
// The limit belongs to the runtime and to no single request. A script that
// serves is still one script, and set_time_limit inside a handler moves that
// script's deadline, as php does where the request is the script. What a
// request does own is its connection, which EnterRequest scopes.

// maxTimeLimit is the longest limit a script can set, past any real one, and
// it keeps the seconds-to-Duration multiply from wrapping a large argument
// into a short limit.
const maxTimeLimit = 100 * 365 * 24 * time.Hour

// SetTimeLimit bounds how long the script may run, restarting the clock the
// way a second set_time_limit call does. Zero removes the limit, maxTimeLimit
// caps a longer one, and the timer cancels the context bindings hold.
func (rt *Runtime) SetTimeLimit(limit time.Duration) {
	if limit < 0 {
		limit = 0
	}
	rt.timeLimit = min(limit, maxTimeLimit)
	rt.armTimer()
}

// TimeLimit answers the limit in force, zero for none.
func (rt *Runtime) TimeLimit() time.Duration { return rt.timeLimit }

// SetIgnoreUserAbort decides whether the client going away ends the script.
func (rt *Runtime) SetIgnoreUserAbort(enable bool) {
	rt.ignoreAbort = enable
}

// IgnoreUserAbort reports whether the run carries on through a disconnect.
func (rt *Runtime) IgnoreUserAbort() bool { return rt.ignoreAbort }

// ConnectionAborted reports that the client closed the connection.
func (rt *Runtime) ConnectionAborted() bool {
	return ended(rt.clientDone)
}

// ClientContext answers the connection this run serves: the request inside a
// handler, and the host's context outside one. A binding that should
// end when the client leaves waits on this; one that should end when the script
// does waits on Runtime.Context.
func (rt *Runtime) ClientContext() context.Context {
	if rt.client != nil {
		return rt.client
	}
	return rt.Context()
}

// EnterRequest records the connection of one request for the length of it,
// and returns the function that puts the previous one back.
func (rt *Runtime) EnterRequest(ctx context.Context) func() {
	saved := rt.BeginRequest(ctx)
	return func() { rt.EndRequest(saved) }
}

// watchEnd answers the channel that closes when ctx ends, nil for a context
// that cannot end.
//
// One read takes the channel and the runtime keeps it and the per-statement
// check is a non-blocking select on a field and no call through a context. A
// channel and not a flag an AfterFunc sets, because an AfterFunc runs on
// its own goroutine: a sleep that returned the instant its context ended would
// reach the next statement ahead of the flag write.
func watchEnd(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}

// ended reports a watched channel having closed, without blocking on one that
// has not.
func ended(done <-chan struct{}) bool {
	if done == nil {
		return false
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// bindContext builds the context this session's bindings take. SetContext
// calls it and no other code does and a binding can block on it
// across a script changing its own limit.
//
// It does not cancel the one it replaces. A host layering onto the context it
// already gave - runner.Context.Register does exactly that, seeding the request
// onto rt.Context() - would otherwise have its new context cancelled through
// the old one it derives from, and every script would end before its first
// statement. resetLimits drops the old one with the program.
func (rt *Runtime) bindContext() {
	base := rt.host
	if base == nil {
		base = context.Background()
	}
	rt.ctx, rt.stopCtx = context.WithCancel(base)
	rt.ctxDone = rt.ctx.Done()
	// Whether the host can end the run. Not whether the wrapper above has a
	// Done channel, which it always does: the timer ends the run through
	// timedOut, and a command line script with no limit and no request must
	// check nothing per statement.
	rt.watching = base.Done() != nil
	rt.refreshDeadlineArmed()
}

// armTimer restarts the clock. The timer sets the flag before it cancels the
// context and a check that finds the context ended can tell whether the limit
// did it.
func (rt *Runtime) armTimer() {
	if rt.scriptTimer != nil {
		rt.scriptTimer.Stop()
		rt.scriptTimer = nil
	}
	rt.timedOut.Store(false)
	if rt.timeLimit <= 0 {
		rt.refreshDeadlineArmed()
		return
	}
	rt.scriptTimer = time.AfterFunc(rt.timeLimit, func() {
		rt.timedOut.Store(true)
		if rt.stopCtx != nil {
			rt.stopCtx()
		}
	})
	rt.refreshDeadlineArmed()
}

// checkDeadline reports the script having run out of time, the handler having
// run out of its own, or the client having gone away.
func (rt *Runtime) checkDeadline() error {
	if rt.timedOut.Load() {
		return &TimeLimitError{Limit: rt.timeLimit}
	}
	if rt.ignoreAbort {
		return nil
	}
	// The connection this run answers, and the context the host gave the
	// script, which for a served request is the same thing one level up.
	if ended(rt.clientDone) || (rt.watching && ended(rt.ctxDone)) {
		return &TimeLimitError{Aborted: true}
	}
	return nil
}

// watchingDeadline reports whether anything can stop this run. A field and no
// computation, which whatever changes one of its inputs refreshes, because the
// statement loop reads it on every statement.
func (rt *Runtime) watchingDeadline() bool { return rt.deadlineArmed }

// refreshDeadlineArmed recomputes it. Called by everything that arms or drops a
// timer, changes the connection, or rebuilds the context - a limit a script
// sets halfway through its own statement list has to reach the rest of that
// list.
func (rt *Runtime) refreshDeadlineArmed() {
	rt.deadlineArmed = rt.watching || rt.clientDone != nil || rt.scriptTimer != nil
}

// suspendDeadline turns the clock off and answers the function that turns it
// back on, for the shutdown pass: a script is there because it ran out of time
// as often as because it finished, and a callback registered to close what it
// opened has to be able to run. php resets the timer for its shutdown functions
// for the same reason.
func (rt *Runtime) suspendDeadline() func() {
	timedOut := rt.timedOut.Load()
	watching, clientDone := rt.watching, rt.clientDone

	rt.timedOut.Store(false)
	rt.watching, rt.clientDone = false, nil
	rt.refreshDeadlineArmed()

	return func() {
		rt.timedOut.Store(timedOut)
		rt.watching, rt.clientDone = watching, clientDone
		rt.refreshDeadlineArmed()
	}
}

// resetLimits returns the deadline and the connection to what a fresh runtime
// has, for a host that reuses one across programs. A limit one program set is
// not the next one's, and a timer left armed would end a program that never
// set a limit.
func (rt *Runtime) resetLimits() {
	// Only a program that armed something needs its context replaced. One that
	// set no limit left the context untouched, and rebuilding it would cost an
	// allocation per run for nothing; the budget in
	// flatstack.TestFlatstackPrecompiledAllocationBudget is what holds that.
	used := rt.scriptTimer != nil || rt.timedOut.Load()
	rt.releaseDeadline()
	if used && rt.stopCtx != nil {
		// The program is over and this drops the context it holds and builds
		// a fresh one from the same host: a runtime the host reuses without
		// setting a context again must not start the next program on a
		// cancelled one.
		rt.stopCtx()
		rt.stopCtx = nil
		rt.bindContext()
	}
	rt.timeLimit = 0
	rt.ignoreAbort = false
	rt.timedOut.Store(false)
	rt.client = rt.host
	rt.clientDone = watchEnd(rt.client)
	rt.inRequest = false
	rt.refreshDeadlineArmed()
}

// releaseDeadline stops the timers a limit installed and a host reusing a
// runtime does not leave one alive per program until it fires.
func (rt *Runtime) releaseDeadline() {
	if rt.scriptTimer != nil {
		rt.scriptTimer.Stop()
		rt.scriptTimer = nil
	}
	rt.refreshDeadlineArmed()
}
