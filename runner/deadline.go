package runner

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// This file is the execution deadline and the client connection, which are two
// things a script wants to end for and are kept apart on purpose.
//
// A Runtime holds three contexts. host is what the host handed it. ctx is what
// the VM checks and every binding is handed, host with the time limit on it;
// it is rebuilt only when the limit or the abort policy changes, so a binding
// that is blocked on it - HTTP\Mux::serve, which is the script's whole run -
// keeps watching the same one. client is the connection being answered right
// now, which is host until a handler enters a request, and is what
// connection_aborted() reads.
//
// The client is checked rather than merged into ctx, because merging would mean
// rebuilding ctx per request and cancelling the one serve() is waiting on.

// TimeLimitError ends a script that ran past set_time_limit, or whose client
// went away while it was not ignoring that.
//
// It unwinds like the memory limit does, so an enclosing try still catches it.
// A host distinguishes the two causes through Aborted, because they call for
// different answers: a timeout is the site's problem and a disconnect is not.
type TimeLimitError struct {
	// Limit is the limit that was set, zero when the client went away first.
	Limit time.Duration

	// Aborted reports that the client closed the connection.
	Aborted bool
}

func (e *TimeLimitError) Error() string {
	if e.Aborted {
		return "Script ended: the client closed the connection"
	}
	return fmt.Sprintf("Maximum execution time of %d seconds exceeded", int(e.Limit.Seconds()))
}

// SetTimeLimit bounds how long the script may run, restarting the clock from
// now the way a second set_time_limit call does. Zero removes the limit.
//
// The limit is a deadline on the context the VM checks and bindings are handed,
// so it ends a Go call that is waiting as well as a PHP loop that is spinning.
func (rt *Runtime) SetTimeLimit(limit time.Duration) {
	if limit < 0 {
		limit = 0
	}
	rt.timeLimit = limit
	rt.deriveContext()
}

// TimeLimit answers the limit in force, zero for none.
func (rt *Runtime) TimeLimit() time.Duration { return rt.timeLimit }

// SetIgnoreUserAbort decides whether the client going away ends the script.
//
// Off, which is the default, the disconnect stops the script where it next
// looks. On, the script runs to its own end and asks ConnectionAborted when it
// wants to know. A time limit still applies either way: ignoring the client is
// not permission to run forever.
// It is a flag rather than a change to the context, because the context is
// what a blocked binding is already waiting on: rebuilding it here would end
// HTTP\Mux::serve the moment a handler called this. What it costs is that a Go
// call made after the client left still sees the disconnect on its context and
// may return early; the script itself continues, which is what was asked for.
func (rt *Runtime) SetIgnoreUserAbort(enable bool) {
	rt.ignoreAbort = enable
}

// IgnoreUserAbort reports whether a disconnect is being ignored.
func (rt *Runtime) IgnoreUserAbort() bool { return rt.ignoreAbort }

// ConnectionAborted reports that the client closed the connection.
//
// It answers for the connection being served rather than for the context the
// VM checks, so it keeps answering after ignore_user_abort detached the two,
// which is the only arrangement in which a script is still running to ask.
func (rt *Runtime) ConnectionAborted() bool {
	return ended(rt.clientDone)
}

// EnterRequest records the connection of one request while it is answered, and
// returns the function that puts the previous one back.
//
// It is for a host answering several requests on one runtime, which HTTP\Mux
// does: connection_aborted() then reports the client that is waiting rather
// than whatever started the script, and a disconnect ends the handler unless it
// asked to ignore that.
//
// Only the connection moves. The time limit bounds the script, and for a script
// that is a server the script outlives every request in it.
func (rt *Runtime) EnterRequest(ctx context.Context) func() {
	previous, previousDone, previousWatch := rt.client, rt.clientDone, rt.watchClient
	previousIgnore := rt.ignoreAbort

	rt.client = ctx
	// Off for every request, so a handler that called ignore_user_abort does
	// not decide it for the next one. php settles it per request too, because
	// there a request is a script and the setting dies with it.
	rt.ignoreAbort = false
	rt.watchClient, rt.clientDone = watchEnd(ctx)

	return func() {
		rt.client, rt.clientDone, rt.watchClient = previous, previousDone, previousWatch
		rt.ignoreAbort = previousIgnore
	}
}

// watchEnd answers the channel that closes when ctx ends, and whether ctx can
// end at all.
//
// The channel is read once and kept, so the per-statement check is a
// non-blocking select on a field rather than a call through a context. It has
// to run on every statement: a script that slept past its limit and then does
// three more things has to be stopped at the first of them, and a counter that
// fires every couple of hundred statements would let all three through.
//
// A channel rather than a flag an AfterFunc sets, because an AfterFunc runs on
// its own goroutine: a sleep that returned the instant its context ended would
// reach the next statement before the flag was written, and the script would
// carry on past a limit it had already passed.
func watchEnd(ctx context.Context) (bool, <-chan struct{}) {
	if ctx == nil {
		return false, nil
	}
	done := ctx.Done()
	return done != nil, done
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

// deriveContext rebuilds the context the VM checks from the one the host set.
//
// Called only by what changes an input to it: the host's context, the limit, or
// whether a disconnect counts. Not per request - a rebuild cancels the context
// the previous one produced, and a script serving requests is blocked on it.
func (rt *Runtime) deriveContext() {
	if rt.stopDeadline != nil {
		rt.stopDeadline()
		rt.stopDeadline = nil
	}

	base := rt.host
	if base == nil {
		base = context.Background()
	}
	if rt.timeLimit > 0 {
		rt.ctx, rt.stopDeadline = context.WithTimeout(base, rt.timeLimit)
	} else {
		rt.ctx = base
	}
	// A context that cannot end is not worth a look per statement, which is
	// what a command line run with no limit has.
	rt.watching, rt.ctxDone = watchEnd(rt.ctx)
}

// checkDeadline reports the script having run out of time, or its client having
// gone away, as the error that unwinds it.
func (rt *Runtime) checkDeadline() error {
	if rt.watching && ended(rt.ctxDone) {
		// The deadline firing is the limit; anything else is the host's context
		// ending, which for a served request is the client going away and is
		// reported as what it is rather than as a limit never reached.
		if errors.Is(rt.ctx.Err(), context.DeadlineExceeded) {
			return &TimeLimitError{Limit: rt.timeLimit}
		}
		if !rt.ignoreAbort {
			return &TimeLimitError{Aborted: true}
		}
	}
	// The connection being answered, which is the request inside a handler and
	// is not in ctx: see the file comment.
	if rt.watchClient && !rt.ignoreAbort && ended(rt.clientDone) {
		return &TimeLimitError{Aborted: true}
	}
	return nil
}

// resetLimits returns the deadline and the connection to what a fresh runtime
// has, for a host that reuses one across programs. A limit one program set is
// not the next one's limit, and a connection that went away belonged to the
// program that was answering it; leaving either behind ends the next program
// before its first statement.
func (rt *Runtime) resetLimits() {
	rt.releaseDeadline()
	rt.timeLimit = 0
	rt.ignoreAbort = false
	rt.client = rt.host
	rt.watchClient, rt.clientDone = watchEnd(rt.client)
	rt.deriveContext()
}

// releaseDeadline drops the timer a limit installed, so a host reusing a
// runtime does not leave one alive per session until it fires.
func (rt *Runtime) releaseDeadline() {
	if rt.stopDeadline != nil {
		rt.stopDeadline()
		rt.stopDeadline = nil
	}
}
