package server

import (
	"context"

	"github.com/titpetric/phpscript/runner"
)

// lifecycleTracker records work that arrives from somewhere other than a request.
type lifecycleTracker interface {
	TrackLifecycle(ctx context.Context, name, filename string, run func(context.Context) error) error
}

// nonFatal wraps module so a startup failure reaches the observers and the caller sees nil.
func nonFatal(module LifecycleModule, observers []runner.Observer) LifecycleModule {
	return &nonFatalModule{LifecycleModule: module, observers: observers}
}

type nonFatalModule struct {
	LifecycleModule
	observers []runner.Observer
}

// Start runs the wrapped module and reports its failure by trace, always returning nil.
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
