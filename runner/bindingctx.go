package runner

import (
	"context"
	"time"

	"github.com/titpetric/phpscript/telemetry"
)

// bindingCtx holds one binding call's frame, environment and source location.
type bindingCtx struct {
	context.Context

	scope    *Scope
	env      map[string]string
	filename string
	line     int

	// session is the runtime's own context when the embedded parent is the
	// request's: the session chain holds the active span and the request
	// record Context.Register seeded, while the parent holds the values,
	// the deadline and the cancellation of the request in flight. Nil when
	// the parent is the session context already.
	session context.Context

	// detached drops the parent's cancellation and deadline while keeping its
	// values. The flag spells ignore_user_abort(true) for a binding: the
	// disconnect must not end work the handler said to finish, and what
	// middleware attached still reaches it.
	detached bool
}

// Value answers the field keys, then the session chain, then the parent.
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

// Done answers the parent's channel, nil when detached.
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

// bindingContext builds a binding's context, detachment following ignore_user_abort.
func (rt *Runtime) bindingContext(scope *Scope) context.Context {
	c := &bindingCtx{Context: rt.ctx, scope: scope, env: rt.Env}
	if rt.inRequest && rt.client != nil {
		c.Context, c.session = rt.client, rt.ctx
		c.detached = rt.ignoreAbort
	}
	rt.fillSource(scope, c)
	return c
}

// fillSource records the file and the current line on the context.
func (rt *Runtime) fillSource(scope *Scope, c *bindingCtx) {
	if filename, ok := scope.Get("__FILE__"); ok {
		c.filename, _ = filename.(string)
	} else {
		c.filename = rt.entrypoint
	}
	c.line = rt.currentSourceLine()
}
