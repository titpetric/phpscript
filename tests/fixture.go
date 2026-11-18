package tests

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing/fstest"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/coverage"
)

// FixtureRequest contains request data exposed to PHP as superglobals.
type FixtureRequest struct {
	Args    any               `yaml:"args"` // map[string]string or []string or scalar
	Get     map[string]string `yaml:"get"`
	Post    map[string]string `yaml:"post"`
	Cookie  map[string]string `yaml:"cookie"`
	Env     map[string]string `yaml:"env"`
	Headers map[string]string `yaml:"headers"`
	Stdin   string            `yaml:"stdin"`
	Body    string            `yaml:"body"`
}

// FixtureResponse contains expected response assertions.
type FixtureResponse struct {
	Headers map[string]string `yaml:"headers"`
}

// Fixture is the parsed representation of a .phpt test file.
type Fixture struct {
	Name        string          `yaml:"name"`
	Description string          `yaml:"description"`
	Error       string          `yaml:"error"`   // optional: expected uncaught error substring
	Stdin       string          `yaml:"stdin"`   // optional: top-level stdin contents
	Options     runner.Options  `yaml:"options"` // optional: runtime options for both engines (memory_limit, ...)
	Runtime     string          `yaml:"runtime"` // optional: the runtime namespace, phpscript or go
	Root        string          `yaml:"root"`    // optional: include root, relative to the fixture's own directory
	Serial      bool            `yaml:"serial"`  // optional: do not overlap this fixture with peers in its area
	Runner      FixtureRunners  `yaml:"runner"`  // optional: runners the fixture opts out of
	Request     FixtureRequest  `yaml:"request"`
	Response    FixtureResponse `yaml:"response"`

	PHP      string `yaml:"-"`
	Expected string `yaml:"-"`
	Path     string `yaml:"-"`

	appRoot     string
	cacheScope  string
	includes    []string
	database    model.DatabaseProvider
	mail        model.MailProvider
	coverage    *coverage.Collector
	rootFS      fs.FS
	sources     fstest.MapFS
	rootKey     string
	runtimeKeyS string
	mu          sync.Mutex
	prog        *model.Program
	interp      *runner.Runtime
	flatRT      *flatstack.Runtime
	flatExpr    *flatstack.ExprCache
	flatChecked bool
	flatErr     error
}

// SetRootFS binds the filesystem a fixture's includes resolve against.
func (f *Fixture) SetRootFS(root fs.FS) {
	f.rootFS = root
}

// sourceName names the file the runtime reads, keeping the whole path for __DIR__.
func (f *Fixture) sourceName() string {
	name := fsName(filepath.ToSlash(f.Path))
	// A .php fixture is already a file the runtime can open, with a name the
	// runtime reads back from __FILE__. Only a .phpt needs an invented name.
	if path.Ext(name) == ".php" {
		return name
	}
	return name + ".php"
}

// fsName turns a caller-spelled path into an fs.FS path.
func fsName(p string) string {
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "/")
	for strings.HasPrefix(p, "../") {
		p = strings.TrimPrefix(p, "../")
	}
	if p == ".." || p == "." || p == "" || !fs.ValidPath(p) {
		return path.Base(p)
	}
	return p
}

// sourceFS lays the fixture bodies over root, above the application files.
func (f *Fixture) sourceFS(root fs.FS) fs.FS {
	sources := f.sources
	if sources == nil {
		sources = fstest.MapFS{f.sourceName(): &fstest.MapFile{Data: []byte(f.PHP)}}
	}
	return unionFS{root, sources}
}

// SetAppRoot names the application root autoload and includes resolve against.
func (f *Fixture) SetAppRoot(root string, includes ...string) {
	f.appRoot = root
	f.includes = includes
}

// SetDatabase names the connections this fixture's bindings resolve through.
func (f *Fixture) SetDatabase(provider model.DatabaseProvider) {
	f.database = provider
}

// SetMail names the mail servers this fixture's bindings resolve through.
func (f *Fixture) SetMail(provider model.MailProvider) {
	f.mail = provider
}

// SetCoverage installs a collector for the runtime runner.
func (f *Fixture) SetCoverage(c *coverage.Collector) {
	f.coverage = c
}

// Coverage returns the collector installed with SetCoverage, or nil.
func (f *Fixture) Coverage() *coverage.Collector {
	return f.coverage
}

// RootDir returns the directory a fixture's includes resolve in and the php
// runner runs in: the fixture's folder, or the folder named with `root:`.
func (f *Fixture) RootDir() string {
	dir := filepath.Dir(f.Path)
	if f.Root == "" {
		return dir
	}
	return filepath.Join(dir, f.Root)
}

// realRoot reports whether the fixture named a directory on disk.
func (f *Fixture) realRoot() bool {
	return f.Root != "" || f.appRoot != ""
}

// unionFS resolves a path against each layer in order, first hit wins.
type unionFS []fs.FS

func (u unionFS) Open(name string) (fs.File, error) {
	var firstErr error
	for _, layer := range u {
		file, err := layer.Open(name)
		if err == nil {
			return file, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return nil, firstErr
}

// includePrelude includes the --include files before the body, skipping a missing one.
func (f *Fixture) includePrelude(rt *runner.Runtime) error {
	for _, name := range f.includes {
		clean := strings.TrimPrefix(path.Clean(filepath.ToSlash(name)), "/")
		if _, err := fs.Stat(rt.FS(), clean); err != nil {
			continue
		}
		if _, err := rt.IncludeFile(clean); err != nil {
			return err
		}
	}
	return nil
}

// runnerOptions returns the fixture's options with the harness-owned fields filled in.
func (f *Fixture) runnerOptions() runner.Options {
	options := f.Options
	if f.Runtime != "" {
		options.Runtime = f.Runtime
	}
	// The php column executes the php cli binary and the runtime column
	// speaks as the same SAPI: a fixture probing php_sapi_name() must read
	// the same answer from both.
	options.SAPI = "cli"
	options.RootFS = f.rootFS
	options.Database = f.database
	options.Mail = f.mail
	options.Precompile = true
	if f.realRoot() {
		// A fixture that names a root needs the real filesystem: it reads a
		// tree phpscript does not embed, a vendor directory being the
		// motivating case.
		options.RootFS = os.DirFS(f.RootDir())
	}
	if f.appRoot != "" {
		// The fixture directory answers first. A fixture's relative include
		// keeps its meaning. The application root answers second, for the
		// prelude include, the files it pulls in, and the autoload folder.
		// Those sit outside every fixture directory.
		options.RootFS = unionFS{os.DirFS(f.RootDir()), os.DirFS(f.appRoot)}
	}
	if options.RootFS == nil {
		options.RootFS = testPHPFS()
	}
	options.RootFS = f.sourceFS(options.RootFS)
	options.Stdin = f.stdin()
	return options
}
