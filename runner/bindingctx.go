package runner

import (
	"context"
	"time"

	"github.com/titpetric/phpscript/telemetry"
)

// bindingCtx is the context one binding call receives: the calling frame, the
// script environment and the source location, carried as fields of one value
// in place of four context.WithValue layers. It embeds the parent, so
// deadlines, cancellation and every other value pass through.
//
// The runtime builds one per call and never pools or reuses it. A binding may
// hold the context past the call - a database driver watching Done() on a
// goroutine of its own does - and a reused value would rebind the parent a
// first holder still reads.
type bindingCtx struct {
	context.Context

	scope    *Scope
	env      map[string]string
	filename string
	line     int

	// session is the runtime's own context when the embedded parent is the
	// request's: the active span and the request record Context.Register
	// seeded live on the session chain, while the parent carries the values,
	// the deadline and the cancellation of the request in flight. Nil when
	// the parent is the session context already.
	session context.Context

	// detached drops the parent's cancellation and deadline while keeping its
	// values. The flag spells ignore_user_abort(true) for a binding: the
	// disconnect must not end work the handler said to finish, and what
	// middleware attached still reaches it.
	detached bool
}

// Value answers the four keys from the fields, the session chain next so a
// span nests below the PHP region that started it, and the parent last.
func (c *bindingCtx) Value(key any) any {
	switch key.(type) {
	case scopeContextKey:
		return c.scope
	case envContextKey:
		return c.env
	}
	if v, ok := telemetry.SourceValue(key, c.filename, c.line); ok {
		return v
	}
	if c.session != nil {
		if v := c.session.Value(key); v != nil {
			return v
		}
	}
	return c.Context.Value(key)
}

// Done answers the parent's channel, or nil when detached, matching
// context.WithoutCancel without the second allocation.
func (c *bindingCtx) Done() <-chan struct{} {
	if c.detached {
		return nil
	}
	return c.Context.Done()
}

// Err follows Done.
func (c *bindingCtx) Err() error {
	if c.detached {
		return nil
	}
	return c.Context.Err()
}

// Deadline follows Done.
func (c *bindingCtx) Deadline() (time.Time, bool) {
	if c.detached {
		return time.Time{}, false
	}
	return c.Context.Deadline()
}

// bindingContext builds the context injected into a binding whose first
// parameter declares one.
//
// The base is the request in flight while one is live, so a value middleware
// attached to r.Context(), its deadline and its cancellation all reach the
// binding. runner/runtime_deadline.go states why rt.ctx itself is never
// rebuilt; deriving a value per call is compatible with a binding parked on
// the one it holds. Outside a request the base is the session context, and
// a handler that called ignore_user_abort(true) keeps the request's values
// and drops its cancellation, so a disconnect does not end binding work the
// handler chose to finish.
func (rt *Runtime) bindingContext(scope *Scope) context.Context {
	c := &bindingCtx{Context: rt.ctx, scope: scope, env: rt.Env}
	if rt.inRequest && rt.client != nil {
		c.Context, c.session = rt.client, rt.ctx
		c.detached = rt.ignoreAbort
	}
	rt.fillSource(scope, c)
	return c
}

// fillSource records the file and the line spans started from the context
// attribute to. The line belongs to the runtime and to no scope: the parser
// compiles __LINE__ to a literal, so no frame carries it, and writing it into
// one per statement for a read back here costs a map write on every statement
// executed. fillSource resolves it here and records nothing, for the reason
// currentStmt states.
func (rt *Runtime) fillSource(scope *Scope, c *bindingCtx) {
	if filename, ok := scope.Get("__FILE__"); ok {
		c.filename, _ = filename.(string)
	} else {
		c.filename = rt.entrypoint
	}
	c.line = rt.currentSourceLine()
}
