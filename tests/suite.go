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
	"strings"
	"sync"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/ps/database"
	"github.com/titpetric/phpscript/stdlib/ps/mail"
)

// SuiteFile is the file a directory of fixtures configures itself with, the
// same name an application root serves under. They share one file: a tree
// serving as site and suite writes one configuration, not two.
const SuiteFile = config.VirtualHostConfigFile

// Suite is a phpscript.yml governing the fixtures below its directory: the
// connections they resolve, the prelude they load, and the hooks the session
// runs around them.
//
// Provider builds the provider once and holds it. Setup and fixtures must
// share a pool. Two providers on one DSN mean two pool caches.
// A schema applied with the first is not in the database the second queries
// when the DSN names no shared file.
type Suite struct {
	// Dir is the suite root in the caller's spelling, the spelling an error
	// names. stdlib.RegisterFS binds the shims to that root.
	Dir string

	// Config is the file read over the configuration the run started with.
	Config config.Config

	// Declared holds the keys the file itself named; a caller can tell a
	// setting the suite named from one inherited from the run.
	Declared map[string]any

	// root is the suite directory as a filesystem. A hook resolves its
	// includes and its migrations against this root, as an application root
	// resolves them when serving.
	root fs.FS

	once     sync.Once
	provider model.DatabaseProvider

	mailOnce sync.Once
	mail     model.MailProvider
}

// LoadSuite reads dir/phpscript.yml off disk over base, or returns nil.
func LoadSuite(dir string, base config.Config, forbidden bool) (*Suite, error) {
	return loadSuite(os.DirFS(dir), dir, filepath.Join(dir, SuiteFile), base, forbidden)
}

// LoadSuiteFS is LoadSuite for a directory inside root, for a tree that is not
// on disk. The Go tests read their embedded fixtures this way.
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

// Provider answers the connections the fixtures this suite root governs resolve:
// those declared under env, or the process environment when the suite
// declared none. No fixture can reach a database its folder did not name.
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

// Mail answers the mail servers the fixtures this suite root governs send
// through: those declared by the suite, or nothing, the rule a virtual host
// already follows. A fixture run delivers nothing - construction is a name
// lookup. The names drive the fixtures.
func (s *Suite) Mail() model.MailProvider {
	if s == nil {
		return nil
	}
	s.mailOnce.Do(func() {
		if !config.Declares(s.Declared, "mail") {
			return
		}
		s.mail = mail.NewProvider(s.Config.Mail)
	})
	return s.mail
}

// Include is the prelude the fixtures this suite root governs load, or "".
func (s *Suite) Include() string {
	if s == nil {
		return ""
	}
	return s.Config.Test.Include
}

// Excluded reports whether path is inside a directory test.exclude names,
// resolved from the suite root.
func (s *Suite) Excluded(path string) bool {
	if s == nil {
		return false
	}
	for _, dir := range s.Config.Test.Exclude {
		if underDir(filepath.Join(s.Dir, dir), path) {
			return true
		}
	}
	return false
}

// underDir reports whether path is dir itself or a file below it.
func underDir(dir, path string) bool {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// Setup names the hook the session runs before the fixtures, or "".
func (s *Suite) Setup() string {
	if s == nil {
		return ""
	}
	return s.Config.Test.Hooks.Setup
}

// Teardown names the hook the session runs afterwards, or "".
func (s *Suite) Teardown() string {
	if s == nil {
		return ""
	}
	return s.Config.Test.Hooks.Teardown
}

// RunHook executes one hook file on a runtime built like a fixture in this
// suite root: the standard library rooted at the suite, the host bindings the
// fixtures use, and the suite's own connections.
//
// The suite root is the runtime's filesystem. A hook names its migrations
// relative to itself, as an application does.
func (s *Suite) RunHook(ctx context.Context, file string, out io.Writer) error {
	clean := path.Clean(filepath.ToSlash(file))

	if _, err := fs.Stat(s.root, clean); err != nil {
		return err
	}

	options := s.Config.Runner
	options.SAPI = "cli"
	options.RootFS = s.root
	options.Database = s.Provider()
	options.Mail = s.Mail()
	options.Precompile = true
	// The prelude is the hook's as much as the fixtures': a hook calling into
	// the application's own helpers needs the autoloader's declarations.
	options.Include = s.Include()

	rt := runner.New(out, options)
	rt.RegisterConstructor("Storage", NewStorage)
	rt.RegisterConstructor("FailStorage", NewFailStorage)
	registerPanicBindings(rt)
	// Register installs the shims rooted at the process working directory;
	// RegisterFS rebinds them to the suite and must run after it.
	stdlib.Register(rt)
	stdlib.RegisterFS(rt, s.Dir)
	rt.FreezeStdlib()
	rt.SetContext(ctx)
	rt.UpdateFilename(clean)

	program, err := rt.LoadFile(clean)
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
// the first failure. A suite that lays down no state leaves its fixtures
// nothing to assert. The run fails once, before a fixture executes,
// and reports the missing table once.
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
// Every hook runs, and the error list holds every failure, without stopping at
// the first. The suites are independent; a caller learns about each.
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
