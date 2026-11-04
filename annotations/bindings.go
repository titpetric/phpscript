package annotations

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// Bindings is the service router a @startup job registers on through
// Runtime\Router::Bind, as a separate platform module.
//
// The platform starts every module before it mounts any, so a job runs a step
// before the router it binds on exists: this takes the registrations in the
// router's place and Mount passes them on, in the order the jobs made them.
// A service binds instead of annotating and the module is registered either
// way, and a host that names no router through WithRouter leaves
// Runtime\Router::Bind the binding that throws.
type Bindings struct {
	Module

	mu        sync.Mutex
	registrar Registrar
	pending   []binding
}

// binding is one registration a job made before the router arrived.
type binding struct {
	method  string
	path    string
	handler http.Handler
}

// NewBindings creates the module, attached to the router WithRouter named.
func NewBindings(options ...Option) *Bindings {
	cfg := newConfig(options...)
	bindings := &Bindings{Module: *newModule(cfg.moduleName("phpbindings"))}
	if cfg.router != nil {
		cfg.router.Attach(bindings)
	}
	return bindings
}

// Handle takes one registration, straight to the router once it is mounted and
// onto the pending list until then.
func (b *Bindings) Handle(method string, path string, handler http.Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.registrar != nil {
		b.registrar.Handle(method, path, handler)
		return
	}
	b.pending = append(b.pending, binding{method: method, path: path, handler: handler})
}

// Mount registers the routes the startup jobs bound, and sends a later
// registration straight to router.
func (b *Bindings) Mount(_ context.Context, router Router) error {
	if router == nil {
		return fmt.Errorf("annotations: nil router")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.registrar = platformRegistrar{Router: router}
	for _, pending := range b.pending {
		b.registrar.Handle(pending.method, pending.path, pending.handler)
	}
	b.pending = nil
	return nil
}
