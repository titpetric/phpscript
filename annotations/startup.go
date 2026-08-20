package annotations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/runner"
)

// Startup runs the @startup jobs of a PHP source tree once, in path order,
// before the platform starts its server.
type Startup struct {
	platform.UnimplementedModule
	root   fs.FS
	config config
}

// NewStartup creates a startup lifecycle module reading jobs from root.
func NewStartup(root fs.FS, options ...Option) *Startup {
	cfg := newConfig(options...)
	return &Startup{
		UnimplementedModule: *platform.NewUnimplementedModule(cfg.moduleName("phpstartup")),
		root:                root,
		config:              cfg,
	}
}

// Start executes every PHP file carrying an @startup annotation and reports
// what failed.
//
// One job failing does not stop the others: the tree's jobs are independent,
// and the errors are joined into the returned one so a caller learns about all
// of them rather than the first. Whether a failure is fatal is the caller's
// decision, not this module's; a server hosting several sites records it and
// carries on, where a single site server may well not.
func (s *Startup) Start(ctx context.Context) error {
	var failures []error
	err := scanner{root: s.root, excluded: s.config.excludedDirs}.walk(func(file string, src []byte) error {
		if !HasStartup(src) {
			return nil
		}
		if err := s.run(ctx, file, src); err != nil {
			failures = append(failures, fmt.Errorf("startup %s: %w", file, err))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return errors.Join(failures...)
}

// run executes one job, tracked as a background trace when an observer records
// lifecycle work: a startup job is not a request, and has no trace to join.
func (s *Startup) run(ctx context.Context, file string, src []byte) error {
	job := func(ctx context.Context) error {
		return s.execute(ctx, file, src)
	}
	for _, observer := range s.config.observers {
		if tracker, ok := observer.(interface {
			TrackLifecycle(context.Context, string, string, func(context.Context) error) error
		}); ok {
			return tracker.TrackLifecycle(ctx, "@startup "+file, file, job)
		}
	}
	return job(ctx)
}

func (s *Startup) execute(ctx context.Context, file string, src []byte) error {
	rt := s.config.newRuntime(ctx, s.root, s.config.output, "cli", func(rt *runner.Runtime) {
		runner.NewContext().Register(rt)
	})

	rt.UpdateFilename(file)
	program, err := rt.Load(string(src))
	if err != nil {
		err = fmt.Errorf("parse %q: %w", file, err)
	}
	if err == nil {
		err = rt.Run(program)
	}
	if exit, ok := runner.IsExit(err); ok && exit.Code == 0 {
		return nil
	}
	return err
}
