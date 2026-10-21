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

// sourceName names the file the runtime reads a fixture's PHP section from.
//
// A fixture is a document and no file the runtime can open, so sourceFS
// serves its body as one. The name carries the .phpt through, which no real
// file in the tree ends in, so it shadows nothing the fixture includes.
//
// It keeps the fixture's whole path and not its base, because the parser
// compiles __DIR__ from the name the runtime reads a file under: served at the
// area root the name has no directory, and a fixture reading __DIR__ would see
// "/" where its own folder belongs.
func (f *Fixture) sourceName() string {
	name := fsName(filepath.ToSlash(f.Path))
	// A .php fixture is already a file the runtime can open, under a name it
	// reads back from __FILE__. Only a .phpt needs one invented for it.
	if path.Ext(name) == ".php" {
		return name
	}
	return name + ".php"
}

// fsName turns a path as the caller spelled it into one an fs.FS accepts,
// keeping as much of the directory as it can.
//
// FindFixtures names a fixture from the working directory, so the path arrives
// absolute or with leading ".." segments, and fs.ValidPath rejects both.
// Dropping the leading segments keeps the folder the fixture sits in, and the
// parser compiles __DIR__ from that.
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

// sourceFS lays the fixture bodies over the tree they belong to, as the second
// layer and not the first: unionFS resolves a name to the first layer that
// opens it and does not merge directory listings, so a leading layer holding
// only the bodies would answer "." with a directory holding only those, and
// scandir would stop seeing the tree.
func (f *Fixture) sourceFS(root fs.FS) fs.FS {
	sources := f.sources
	if sources == nil {
		sources = fstest.MapFS{f.sourceName(): &fstest.MapFile{Data: []byte(f.PHP)}}
	}
	return unionFS{root, sources}
}

// SetAppRoot points the fixture at the application root its autoload folder
// and prelude includes resolve against, none of which has to be there.
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

// RootDir returns the directory a fixture's includes resolve against. That is
// the directory holding it, which is also where the php runner executes, unless
// the fixture named another one with `root:`.
func (f *Fixture) RootDir() string {
	dir := filepath.Dir(f.Path)
	if f.Root == "" {
		return dir
	}
	return filepath.Join(dir, f.Root)
}

// realRoot reports whether the fixture named a directory on disk rather
// than the embedded tree. Such a fixture gets its own include caches: those
// key on the path as the script wrote it, so a fixture reaching a different
// tree must not read back a program cached for the embedded one.
func (f *Fixture) realRoot() bool {
	return f.Root != "" || f.appRoot != ""
}

// unionFS resolves a path against each layer in order, first hit wins: a
// fixture keeps its own directory as the include root while the application
// root the test command ran in answers for everything the fixture directory
// does not hold. fs.Stat and fs.ReadFile both fall back to Open, so Open is
// the whole contract.
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

// includePrelude includes the files the test command named with --include
// before the fixture body runs. It runs every session, because ResetSession
// clears user declarations. It skips a file that is not there: the flag
// names what to load when the application carries it.
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

// runnerOptions returns the fixture's options: frontmatter with the
// harness-owned fields filled in.
func (f *Fixture) runnerOptions() runner.Options {
	options := f.Options
	// The php column executes the php cli binary, so the runtime column
	// speaks as the same SAPI: a fixture probing php_sapi_name() must read
	// the same answer from both.
	options.SAPI = "cli"
	options.RootFS = f.rootFS
	options.Database = f.database
	options.Mail = f.mail
	options.Precompile = true
	if f.realRoot() {
		// A fixture that names a root needs the real filesystem: it reaches
		// for a tree phpscript does not embed, a vendor directory being the
		// motivating case.
		options.RootFS = os.DirFS(f.RootDir())
	}
	if f.appRoot != "" {
		// The fixture directory answers first, so a fixture's own relative
		// include keeps meaning what it always meant; the application root
		// answers second, for the prelude include, the files it pulls in, and
		// the autoload folder, which all sit outside every fixture directory.
		options.RootFS = unionFS{os.DirFS(f.RootDir()), os.DirFS(f.appRoot)}
	}
	if options.RootFS == nil {
		options.RootFS = testPHPFS()
	}
	options.RootFS = f.sourceFS(options.RootFS)
	options.Stdin = f.stdin()
	return options
}
