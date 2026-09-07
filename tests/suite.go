package tests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/database"
)

// SuiteFile is the file a directory of fixtures configures itself with. It is
// the same name an application root serves under, because it is the same file:
// a tree that is both a site and a suite writes one configuration, not two.
const SuiteFile = config.VirtualHostConfigFile

// Suite is one phpscript.yml governing the fixtures below the directory holding
// it: the connections they resolve, the prelude they load, and the hooks the
// session runs around them.
//
// The provider is built once and held, because the setup hook and the fixtures
// have to share a pool. Two providers over one DSN are two pool caches, and a
// schema applied through the first is not in the database the second queries
// when the DSN names no shared file.
type Suite struct {
	// Dir is the suite root as the caller spells it, which is what an error
	// names and what stdlib.RegisterFS is rooted at.
	Dir string

	// Config is the file read over the configuration the run started with.
	Config config.Config

	// Declared holds the keys the file itself named, so a caller can tell a
	// setting the suite asked for from one it inherited.
	Declared map[string]any

	// root is the suite directory as a filesystem. A hook resolves its
	// includes and its migrations against it, the way an application root
	// resolves them under the server.
	root fs.FS

	once     sync.Once
	provider model.DatabaseProvider
}

// LoadSuite reads dir/phpscript.yml off disk over base, and returns nil when
// the directory holds no such file. A directory without one is not a suite
// root; it is a folder of fixtures that configures nothing.
//
// forbidden refuses the keys that describe a whole run rather than one folder.
// It is false for the configuration found at or above the invocation root,
// which is the run's own, and true for one discovered below it.
func LoadSuite(dir string, base config.Config, forbidden bool) (*Suite, error) {
	return loadSuite(os.DirFS(dir), dir, filepath.Join(dir, SuiteFile), base, forbidden)
}

// LoadSuiteFS is LoadSuite over a directory inside root, for a tree that is not
// on disk. The embedded fixtures the Go tests run are read this way.
func LoadSuiteFS(root fs.FS, dir string, base config.Config) (*Suite, error) {
	sub, err := fs.Sub(root, dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	return loadSuite(sub, dir, path.Join(dir, SuiteFile), base, true)
}

func loadSuite(root fs.FS, dir, filename string, base config.Config, forbidden bool) (*Suite, error) {
	data, err := fs.ReadFile(root, SuiteFile)
	if err != nil {
		return nil, nil
	}
	loaded, err := config.LoadTestSuite(dir, filename, data, base, forbidden)
	if err != nil {
		return nil, err
	}
	return &Suite{
		Dir:      loaded.Dir,
		Config:   loaded.Config,
		Declared: loaded.Declared,
		root:     root,
	}, nil
}

// Provider answers the connections the fixtures below this suite root resolve.
//
// A suite that declared no env of its own resolves what the run does, which is
// the process environment: a folder that named no connections is not asking for
// a set of its own. A suite that declared one gets only what it named, the rule
// a virtual host is already held to, so a fixture cannot reach a database its
// folder did not configure.
func (s *Suite) Provider() model.DatabaseProvider {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		if !config.Declares(s.Declared, "env") {
			return
		}
		s.provider = database.New(s.Config.Env)
	})
	return s.provider
}

// Include is the prelude the fixtures below this suite root load, or "".
func (s *Suite) Include() string {
	if s == nil {
		return ""
	}
	return s.Config.Test.Include
}

// Setup names the hook the session runs before the fixtures, or "".
func (s *Suite) Setup() string {
	if s == nil {
		return ""
	}
	return s.Config.Test.Hooks.Setup
}

// Teardown names the hook the session runs after them, or "".
func (s *Suite) Teardown() string {
	if s == nil {
		return ""
	}
	return s.Config.Test.Hooks.Teardown
}

// RunHook executes one hook file on a runtime carrying what a fixture below
// this suite root carries: the standard library rooted at the suite, the host
// bindings the fixtures use, and the suite's own connections.
//
// The suite root is the runtime's filesystem, so a hook names its migrations
// the way an application does, relative to itself. Output goes to out, which a
// caller not reporting the hook may discard.
func (s *Suite) RunHook(ctx context.Context, file string, out io.Writer) error {
	clean := path.Clean(filepath.ToSlash(file))

	src, err := fs.ReadFile(s.root, clean)
	if err != nil {
		return err
	}

	options := s.Config.Runner
	options.SAPI = "cli"
	options.RootFS = s.root
	options.Database = s.Provider()
	// The prelude is the hook's as much as the fixtures': a hook calling into
	// the application's own helpers needs what the autoloader declares.
	options.Include = s.Include()

	rt := runner.New(out, options)
	rt.RegisterConstructor("Storage", NewStorage)
	rt.RegisterConstructor("FailStorage", NewFailStorage)
	registerPanicBindings(rt)
	// Register installs the shims rooted at the process working directory;
	// RegisterFS rebinds them to the suite, so it has to run after it.
	stdlib.Register(rt)
	stdlib.RegisterFS(rt, s.Dir)
	rt.FreezeStdlib()
	rt.SetContext(ctx)
	rt.UpdateFilename(clean)

	program, err := rt.Load(string(src))
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if err := rt.Run(program); err != nil {
		if exit, ok := runner.IsExit(err); ok && exit.Code == 0 {
			return nil
		}
		return err
	}
	return nil
}

// RunSetup runs the setup hooks of every suite, outermost first, and stops at
// the first failure. A suite whose state was not laid down has nothing for its
// fixtures to assert against, so the run fails before one executes rather than
// reporting the same missing table once per fixture.
func RunSetup(ctx context.Context, suites []*Suite, out io.Writer) error {
	for _, suite := range suites {
		hook := suite.Setup()
		if hook == "" {
			continue
		}
		if err := suite.RunHook(ctx, hook, out); err != nil {
			return fmt.Errorf("test setup %s: %w", filepath.Join(suite.Dir, hook), err)
		}
	}
	return nil
}

// RunTeardown runs the teardown hooks in reverse, whatever the fixtures did.
//
// Every hook runs and every failure is reported, rather than the run stopping
// at the first: the suites are independent, and a caller learns about all of
// them instead of the one that happened to be innermost.
func RunTeardown(ctx context.Context, suites []*Suite, out io.Writer) error {
	var failures []error
	for i := len(suites) - 1; i >= 0; i-- {
		suite := suites[i]
		hook := suite.Teardown()
		if hook == "" {
			continue
		}
		if err := suite.RunHook(ctx, hook, out); err != nil {
			failures = append(failures, fmt.Errorf("test teardown %s: %w", filepath.Join(suite.Dir, hook), err))
		}
	}
	return errors.Join(failures...)
}
