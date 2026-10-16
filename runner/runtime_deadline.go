package runner

import (
	"context"
	"time"
)

// This file is the execution deadline and the client connection: two things a
// script ends for, kept apart because they end it differently.
//
// The context a binding is handed is built once per session and never rebuilt.
// A time limit is a timer that cancels it, and changing the limit resets the
// timer and derives no new context, because a binding that blocks -
// HTTP\Server::wait, which is a serving script's whole run - is parked on the
// context it holds and a rebuild would end it. A handler calling
// set_time_limit is the ordinary way to reach that.
//
// The connection is checked beside the context and never merged into it, for
// the same reason: a request is one of many and the context belongs to the
// script.
//
// The limit belongs to the runtime and to no single request. A script that serves is still
// one script, and set_time_limit inside a handler moves that script's deadline,
// as php does where the request is the script. What a request does
// own is its connection, scoped by EnterRequest.

// maxTimeLimit is the longest limit a script can set. It is past any real
// one and keeps the seconds-to-Duration multiply from wrapping a large argument
// into a short limit.
const maxTimeLimit = 100 * 365 * 24 * time.Hour

// SetTimeLimit bounds how long the script may run, restarting the clock from
// now the way a second set_time_limit call does. Zero removes the limit, and a
// limit past maxTimeLimit is held there.
//
// It is the runtime's limit wherever it is called from, including a handler of
// the script's own HTTP server: a serving script is one script and this is its
// deadline. The context every binding holds is cancelled with it, so a blocked
// Go call ends too, and a handler extending the limit extends the server rather
// than replacing the context the server is parked on.
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
//
// Off, which is the default, the disconnect stops the script where it next
// looks. On, the script runs to its own end and asks ConnectionAborted when it
// reads. A time limit still applies either way: ignoring the client is
// not permission to run forever.
func (rt *Runtime) SetIgnoreUserAbort(enable bool) {
	rt.ignoreAbort = enable
}

// IgnoreUserAbort reports whether a disconnect is being ignored.
func (rt *Runtime) IgnoreUserAbort() bool { return rt.ignoreAbort }

// ConnectionAborted reports that the client closed the connection: the request
// being answered inside a handler, and whatever started the script outside one.
func (rt *Runtime) ConnectionAborted() bool {
	return ended(rt.clientDone)
}

// ClientContext answers the connection being served, which is the request
// inside a handler and the host's context outside one. A binding that should
// end when the client leaves waits on this; one that should end when the script
// does waits on Runtime.Context.
func (rt *Runtime) ClientContext() context.Context {
	if rt.client != nil {
		return rt.client
	}
	return rt.Context()
}

// EnterRequest records the connection of one request while it is answered, and
// returns the function that puts the previous one back.
//
// It is for a host answering several requests on one runtime, which HTTP\Mux
// does. Everything a request can change is saved: the connection
// connection_aborted() reports, whether a disconnect ends it, and the limit,
// because php settles all three per request and there a request is a script.
func (rt *Runtime) EnterRequest(ctx context.Context) func() {
	saved := rt.BeginRequest(ctx)
	return func() { rt.EndRequest(saved) }
}

// RequestState is what BeginRequest saves, held on the caller's frame and
// handed back to EndRequest. A value and no closure, because the restore
// closure EnterRequest builds is one heap allocation per request on the
// serving path.
type RequestState struct {
	client      context.Context
	clientDone  <-chan struct{}
	ignoreAbort bool
	inRequest   bool
}

// BeginRequest is EnterRequest with the saved state as its answer; EndRequest
// puts it back.
func (rt *Runtime) BeginRequest(ctx context.Context) RequestState {
	saved := RequestState{
		client:      rt.client,
		clientDone:  rt.clientDone,
		ignoreAbort: rt.ignoreAbort,
		inRequest:   rt.inRequest,
	}

	rt.client = ctx
	rt.clientDone = watchEnd(ctx)
	// Off for every request, so a handler that ignored a disconnect does not
	// decide it for the next one. The limit is not saved: it is
	// the runtime's and a handler moving it means to move it.
	rt.ignoreAbort = false
	rt.inRequest = true
	rt.refreshDeadlineArmed()
	return saved
}

// EndRequest restores what BeginRequest saved.
func (rt *Runtime) EndRequest(saved RequestState) {
	rt.client, rt.clientDone, rt.ignoreAbort, rt.inRequest = saved.client, saved.clientDone, saved.ignoreAbort, saved.inRequest
	rt.refreshDeadlineArmed()
}

// watchEnd answers the channel that closes when ctx ends, nil for a context
// that cannot end.
//
// The channel is read once and kept, so the per-statement check is a
// non-blocking select on a field and not a call through a context. A
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

// bindContext builds the context this session's bindings are handed. SetContext
// calls it and nothing else does, so a binding can block on it
// across a script changing its own limit.
//
// It does not cancel the one it replaces. A host layering onto the context it
// already gave - runner.Context.Register does exactly that, seeding the request
// onto rt.Context() - would otherwise have its new context cancelled through
// the old one it derives from, and every script would end before its first
// statement. The old one is released with the program, in resetLimits.
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

// armTimer restarts the clock. The flag is set before the context is cancelled,
// so a check that finds the context ended already knows whether the limit is
// why.
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
	// The connection being answered, and the context the host gave the script,
	// which for a served request is the same thing one level up.
	if ended(rt.clientDone) || (rt.watching && ended(rt.ctxDone)) {
		return &TimeLimitError{Aborted: true}
	}
	return nil
}

// watchingDeadline reports whether anything can stop this run. It is a field
// and no computation, refreshed by whatever changes one of its inputs,
// because the statement loop reads it on every statement.
func (rt *Runtime) watchingDeadline() bool { return rt.deadlineArmed }

// refreshDeadlineArmed recomputes it. Called by everything that arms or drops a
// timer, changes the connection, or rebuilds the context - a limit a script
// sets halfway through its own statement list has to be noticed by the rest of
// that list.
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
		// The program is over, so the context it holds is released and a
		// fresh one built from the same host: a runtime the host reuses without
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

// releaseDeadline stops the timers a limit installed, so a host reusing a
// runtime does not leave one alive per program until it fires.
func (rt *Runtime) releaseDeadline() {
	if rt.scriptTimer != nil {
		rt.scriptTimer.Stop()
		rt.scriptTimer = nil
	}
	rt.refreshDeadlineArmed()
}
