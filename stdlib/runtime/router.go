// Package runtime registers the Runtime\* bindings: the handles a script has
// on the process running it. Runtime\Router::Bind routes the service from a
// @startup job.
package runtime

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	phphttp "github.com/titpetric/phpscript/stdlib/http"
)

func init() {
	runner.RegisterBinding(Register)
}

// Registrar binds a handler to an HTTP method and path. The annotations
// package's Registrar is this interface, so either one fits the other.
type Registrar interface {
	Handle(method string, path string, handler http.Handler)
}

// Router is the service router a @startup job reaches through
// Runtime\Router::Bind. A host attaches the router once it is mounted and
// installs the binding on the runtimes of its startup jobs, the only place the
// router is still mutable.
type Router struct {
	mu        sync.Mutex
	registrar Registrar
}

// NewRouter returns a router with nothing attached.
func NewRouter() *Router {
	return &Router{}
}

// Register installs Runtime\Router::Bind as the binding that throws.
func Register(rt *runner.Runtime) {
	// Runtime\Router::Bind routes the service from a @startup job and throws a RuntimeException anywhere else.
	rt.RegisterFunc("Runtime\\Router::Bind", func(any) error {
		return runner.NewRuntimeException("Runtime\\Router::Bind is only available to a @startup job", 0)
	})
}

// Attach sets the registrar a Bind call registers on.
func (r *Router) Attach(registrar Registrar) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registrar = registrar
}

// Install replaces the binding that throws on rt.
func (r *Router) Install(rt *runner.Runtime) {
	rt.RegisterFunc("Runtime\\Router::Bind", func(fn any) error {
		return r.bind(rt, fn)
	})
}

// bind calls fn with a mux that registers straight on the attached registrar.
//
// The mux is only good while fn runs: it keeps no routes, so nothing lists
// them, and a handle on it after fn has returned throws.
func (r *Router) bind(rt *runner.Runtime, fn any) error {
	r.mu.Lock()
	registrar := r.registrar
	r.mu.Unlock()
	if registrar == nil {
		return runner.NewRuntimeException("Runtime\\Router::Bind: no router is mounted", 0)
	}

	callable, err := rt.AsCallable(fn)
	if err != nil {
		return fmt.Errorf("Runtime\\Router::Bind: %w", err)
	}
	open := true
	mux := phphttp.NewMountedMux(rt, func(pattern string, handler http.Handler) error {
		if !open {
			return runner.NewRuntimeException("Runtime\\Router::Bind: the router is not mutable once Bind has returned", 0)
		}
		method, path, ok := strings.Cut(pattern, " ")
		if !ok {
			return runner.NewRuntimeException(fmt.Sprintf("Runtime\\Router::Bind: %q needs a method, as in GET %s", pattern, pattern), 0)
		}
		if _, err := model.ParseRoutePath(path); err != nil {
			return runner.NewRuntimeException(fmt.Sprintf("Runtime\\Router::Bind: %s", err), 0)
		}
		registrar.Handle(method, path, handler)
		return nil
	})
	defer func() { open = false }()
	_, err = callable.Invoke(rt, mux)
	return err
}
