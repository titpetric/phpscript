package annotations

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"sync"
	"time"

	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

// Scheduler runs @schedule jobs for the life of the server. Start returns
// immediately; each job waits until its next due time in its own goroutine.
type Scheduler struct {
	platform.UnimplementedModule
	root   fs.FS
	config config
	now    func() time.Time
}

type scheduledJob struct {
	file string
	src  []byte
	spec Schedule
}

// NewScheduler creates a schedule module reading jobs from root.
func NewScheduler(root fs.FS, options ...Option) *Scheduler {
	cfg := newConfig(options...)
	return &Scheduler{
		UnimplementedModule: *platform.NewUnimplementedModule(cfg.moduleName("phpschedule")),
		root:                root,
		config:              cfg,
		now:                 time.Now,
	}
}

// Start discovers jobs and runs them until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) error {
	var jobs []scheduledJob
	err := scanner{root: s.root, excluded: s.config.excludedDirs}.walk(func(file string, src []byte) error {
		for _, spec := range ParseSchedules(src) {
			jobs = append(jobs, scheduledJob{file: file, src: src, spec: spec})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, job := range jobs {
		go s.loop(ctx, job)
	}
	return nil
}

func (s *Scheduler) loop(ctx context.Context, job scheduledJob) {
	var running sync.Mutex
	next := job.spec.Next(s.now())
	for {
		timer := time.NewTimer(next.Sub(s.now()))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if running.TryLock() {
			s.run(ctx, job)
			running.Unlock()
		}
		next = job.spec.Next(s.now())
	}
}

func (s *Scheduler) run(ctx context.Context, job scheduledJob) {
	name := "@schedule " + job.spec.Raw
	if len(job.spec.Args) > 0 {
		name += " " + job.spec.Args[0]
	}
	name += " " + job.file
	work := func(ctx context.Context) error {
		return s.execute(ctx, job)
	}
	for _, observer := range s.config.observers {
		if tracker, ok := observer.(interface {
			TrackLifecycle(context.Context, string, string, func(context.Context) error) error
		}); ok {
			_ = tracker.TrackLifecycle(ctx, name, job.file, work)
			return
		}
	}
	_ = work(ctx)
}

func (s *Scheduler) execute(ctx context.Context, job scheduledJob) error {
	var buf bytes.Buffer
	out := io.Writer(&buf)
	if s.config.output != nil {
		out = io.MultiWriter(s.config.output, &buf)
	}
	req := runner.NewContext()
	req.Argv = append([]string{job.file}, job.spec.Args...)
	rt := s.config.newRuntime(ctx, s.root, out, "cli", func(rt *runner.Runtime) {
		req.Register(rt)
	})
	rt.UpdateFilename(job.file)
	program, err := rt.Load(string(job.src))
	if err == nil {
		err = rt.Run(program)
	}
	if span := telemetry.SpanFromContext(ctx); span != nil && buf.Len() > 0 {
		span.SetAttribute("output", buf.String())
	}
	if exit, ok := runner.IsExit(err); ok && exit.Code == 0 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("schedule %s: %w", job.file, err)
	}
	return nil
}
