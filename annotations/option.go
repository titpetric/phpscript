package annotations

import (
	"io"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// Option configures how annotated PHP files are discovered and executed. The
// same options apply to Route and Startup: both scan one source tree and run
// PHP files out of it.
type Option func(*config)

// config is the settings shared by Route and Startup.
type config struct {
	rootDir       string
	output        io.Writer
	runnerOptions runner.Options
	flatstack     bool
	observers     []runner.Observer
	runtimeFuncs  []RuntimeFunc
	exprCache     *runner.ExprCache
	excludedDirs  map[string]struct{}
	moduleSuffix  string
}

// moduleName returns the platform module name for base, suffixed when the
// caller asked for one.
func (c config) moduleName(base string) string {
	if c.moduleSuffix == "" {
		return base
	}
	return base + ":" + c.moduleSuffix
}

func newConfig(options ...Option) config {
	cfg := config{excludedDirs: make(map[string]struct{})}
	for _, option := range options {
		option(&cfg)
	}
	return cfg
}

// WithRootDir grants annotated PHP files access to the project directory,
// backing filesystem functions like fopen().
func WithRootDir(dir string) Option {
	return func(c *config) {
		c.rootDir = dir
	}
}

// WithOutput sets where a startup job writes its output. Routed endpoints write
// to the HTTP response instead, and ignore this.
func WithOutput(out io.Writer) Option {
	return func(c *config) {
		c.output = out
	}
}

// WithRunnerOptions configures the runtimes created for annotated PHP files.
func WithRunnerOptions(options runner.Options) Option {
	return func(c *config) {
		c.runnerOptions = options
	}
}

// WithFlatstack enables the flat bytecode runtime with interpreter fallback.
func WithFlatstack(enabled bool) Option {
	return func(c *config) {
		c.flatstack = enabled
	}
}

// WithObservers attaches runtime observers to every annotated PHP file.
func WithObservers(observers ...runner.Observer) Option {
	return func(c *config) {
		for _, observer := range observers {
			if observer != nil {
				c.observers = append(c.observers, observer)
			}
		}
	}
}

// WithRuntimeFunc registers fn to customize each runtime after the standard
// library and the request context have been installed on it.
func WithRuntimeFunc(fn RuntimeFunc) Option {
	return func(c *config) {
		if fn != nil {
			c.runtimeFuncs = append(c.runtimeFuncs, fn)
		}
	}
}

// WithExprCache sets a shared expression cache used by routed endpoints.
func WithExprCache(cache *runner.ExprCache) Option {
	return func(c *config) {
		c.exprCache = cache
	}
}

// WithModuleSuffix distinguishes the platform modules of one source tree from
// another's. A server running several virtual hosts registers a module set per
// site, and platform.Options.Modules addresses modules by name, so the names
// have to differ.
func WithModuleSuffix(suffix string) Option {
	return func(c *config) {
		c.moduleSuffix = strings.TrimSpace(suffix)
	}
}

// WithExcludedDirectory skips a top-level directory while scanning.
func WithExcludedDirectory(name string) Option {
	return func(c *config) {
		name = strings.Trim(name, "/")
		if name != "" {
			c.excludedDirs[name] = struct{}{}
		}
	}
}
