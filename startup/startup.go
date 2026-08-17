// Package startup runs PHP files annotated with @startup during service startup.
package startup

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/composer"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// Module executes annotated PHP files before the platform starts its server.
type Module struct {
	platform.UnimplementedModule
	root          fs.FS
	rootDir       string
	out           io.Writer
	runnerOptions runner.Options
	flatstack     bool
	observers     []runner.Observer
}

// NewModule creates a startup lifecycle module.
func NewModule(root fs.FS, rootDir string, out io.Writer, options runner.Options, flatstack bool, observers ...runner.Observer) *Module {
	return &Module{
		UnimplementedModule: *platform.NewUnimplementedModule("phpstartup"),
		root:                root,
		rootDir:             rootDir,
		out:                 out,
		runnerOptions:       options,
		flatstack:           flatstack,
		observers:           observers,
	}
}

// Start finds and executes every PHP file containing an @startup comment.
func (m *Module) Start(ctx context.Context) error {
	if m.root == nil {
		return fmt.Errorf("startup: nil root filesystem")
	}
	return fs.WalkDir(m.root, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			// A composer dependency does not get to run at startup.
			if entry.Name() == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".php" {
			return nil
		}
		src, err := fs.ReadFile(m.root, path)
		if err != nil {
			return err
		}
		if !Annotated(src) {
			return nil
		}
		if err := m.run(ctx, path, src); err != nil {
			return fmt.Errorf("startup %s: %w", path, err)
		}
		return nil
	})
}

// Annotated reports whether src carries an @startup comment, marking a file
// the server executes once before it listens. `phpscript list` uses it to show
// startup files alongside routed ones.
func Annotated(src []byte) bool {
	for line := range strings.SplitSeq(string(src), "\n") {
		text := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(text, "//"):
			text = strings.TrimPrefix(text, "//")
		case strings.HasPrefix(text, "#"):
			text = strings.TrimPrefix(text, "#")
		case strings.HasPrefix(text, "/*"):
			text = strings.TrimPrefix(text, "/*")
		case strings.HasPrefix(text, "*"):
			text = strings.TrimPrefix(text, "*")
		default:
			continue
		}
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "*/"))
		fields := strings.Fields(text)
		if len(fields) > 0 && strings.TrimSuffix(fields[0], ":") == "@startup" {
			return true
		}
	}
	return false
}

func (m *Module) run(ctx context.Context, path string, src []byte) error {
	run := func(ctx context.Context) error {
		return m.runPHP(ctx, path, src)
	}
	for _, observer := range m.observers {
		if tracker, ok := observer.(interface {
			TrackLifecycle(context.Context, string, string, func(context.Context) error) error
		}); ok {
			return tracker.TrackLifecycle(ctx, "@startup "+path, path, run)
		}
	}
	return run(ctx)
}

func (m *Module) runPHP(ctx context.Context, path string, src []byte) error {
	options := m.runnerOptions
	options.RootFS = m.root
	options.SAPI = "cli"
	newRuntime := runner.New
	if m.flatstack {
		newRuntime = runner.NewFlatStack
	}
	rt := newRuntime(m.out, options)
	rt.SetContext(ctx)
	for _, observer := range m.observers {
		rt.Observe(observer)
	}
	stdlib.Register(rt)
	if m.rootDir != "" {
		stdlib.RegisterFS(rt, m.rootDir)
	}
	runner.NewContext().Register(rt)
	if err := composer.Register(rt, m.root, "."); err != nil {
		return err
	}

	rt.UpdateFilename(path)
	program, err := rt.Load(string(src))
	if err != nil {
		err = fmt.Errorf("parse %q: %w", path, err)
	}
	if err == nil {
		err = rt.Run(program)
	}
	if exit, ok := runner.IsExit(err); ok && exit.Code == 0 {
		return nil
	}
	return err
}
