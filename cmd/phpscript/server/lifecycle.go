package server

import (
	"context"

	"github.com/titpetric/phpscript/runner"
)

// lifecycleTracker is the part of the telemetry observer that records work
// arriving from somewhere other than a request. Start matches it structurally,
// the way annotations matches it and a host with telemetry off passes no
// observer and nothing here has to know.
type lifecycleTracker interface {
	TrackLifecycle(ctx context.Context, name, filename string, run func(context.Context) error) error
}

// nonFatal returns module with its startup failure recorded and never
// returned.
//
// The platform aborts the process when a module's Start returns an error, which
// is right for a server that is one application and wrong for a server that
// holds more than one: one site's broken @startup job would take every other
// site down with it. The module still reports what failed, and reporting is
// what this uses; deciding that the failure is survivable belongs here, at the
// composition site, not in the module.
func nonFatal(module LifecycleModule, observers []runner.Observer) LifecycleModule {
	return &nonFatalModule{LifecycleModule: module, observers: observers}
}

type nonFatalModule struct {
	LifecycleModule
	observers []runner.Observer
}

// Start runs the wrapped module and records a failure on a trace of its own.
// There is no request to record onto and the error reaches the debug front end
// as lifecycle work, where an operator looks for it.
func (m *nonFatalModule) Start(ctx context.Context) error {
	run := func(ctx context.Context) error {
		return m.LifecycleModule.Start(ctx)
	}
	for _, observer := range m.observers {
		if tracker, ok := observer.(lifecycleTracker); ok {
			_ = tracker.TrackLifecycle(ctx, m.Name(), "", run)
			return nil
		}
	}
	_ = run(ctx)
	return nil
}
