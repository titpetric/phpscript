package annotations

import (
	"io"
	"net/http"
	"strings"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/coverage"
)

// Option configures how the scanner finds annotated PHP files and runs them. The
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
	includeCache  *runner.IncludeCache
	excludedDirs  map[string]struct{}
	moduleSuffix  string
	errorPages    ErrorPageFunc
	coverage      *coverage.Aggregator
}

// ErrorPageFunc renders a host's own page for an error response and reports
// whether it did. status is the status the endpoint ended on and notes is what
// went wrong, empty unless the endpoint failed.
//
// A routed endpoint is a file outside the document root, so it has no error
// page of its own to find; the host that owns the document root supplies one of
// these. A false answer, and a nil func, leave the response to the endpoint.
type ErrorPageFunc func(w http.ResponseWriter, r *http.Request, status int, notes string) bool

// moduleName returns the platform module name for base, suffixed when the
// caller named one.
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

// WithRuntimeFunc registers fn to customize each runtime once the standard
// library and the request context sit on it.
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

// WithIncludeCache sets a shared include cache, which belongs to one source
// tree: include paths resolve against one filesystem root.
func WithIncludeCache(cache *runner.IncludeCache) Option {
	return func(c *config) {
		c.includeCache = cache
	}
}

// WithModuleSuffix distinguishes the platform modules of one source tree from
// another's, as a server of two virtual hosts registers both.
func WithModuleSuffix(suffix string) Option {
	return func(c *config) {
		c.moduleSuffix = strings.TrimSpace(suffix)
	}
}

// WithErrorPages lets a host answer a routed endpoint's error response with a
// page of its own. Startup and scheduled jobs answer to nobody and ignore it.
func WithErrorPages(fn ErrorPageFunc) Option {
	return func(c *config) {
		c.errorPages = fn
	}
}

// WithCoverage counts the statements annotated PHP files execute, into an
// aggregator that outlives the runs.
func WithCoverage(aggregator *coverage.Aggregator) Option {
	return func(c *config) {
		c.coverage = aggregator
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
