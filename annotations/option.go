package annotations

import (
	"io"
	"net/http"
	"strings"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/coverage"
	stdruntime "github.com/titpetric/phpscript/stdlib/ps/runtime"
)

// Option configures how the scanner finds annotated PHP files and runs those
// files. The same options apply to Route and Startup: both scan a source tree
// and run PHP files from it.
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
	router        *stdruntime.Router
}

// ErrorPageFunc renders a host's own page for an error response and reports
// whether it did. status is the status the endpoint ended on; notes describes
// the failure, empty unless the endpoint failed.
//
// A routed endpoint is a file outside the document root and it has no error
// page to find; the host that owns the document root supplies one of these. A
// false answer or a nil func leaves the response to the endpoint.
type ErrorPageFunc func(w http.ResponseWriter, r *http.Request, status int, notes string) bool

// moduleName returns base with the configured module suffix appended.
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

// WithRootDir grants annotated PHP files the project directory, backing filesystem functions like fopen().
func WithRootDir(dir string) Option {
	return func(c *config) {
		c.rootDir = dir
	}
}

// WithOutput sets the output writer for a startup job. Routed endpoints write
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

// WithObservers attaches runtime observers to each annotated PHP file.
func WithObservers(observers ...runner.Observer) Option {
	return func(c *config) {
		for _, observer := range observers {
			if observer != nil {
				c.observers = append(c.observers, observer)
			}
		}
	}
}

// WithRuntimeFunc registers fn to customize each runtime after the stdlib loads.
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
// tree: include paths resolve under one filesystem root.
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
// host page. Startup and scheduled jobs answer to nobody and ignore it.
func WithErrorPages(fn ErrorPageFunc) Option {
	return func(c *config) {
		c.errorPages = fn
	}
}

// WithCoverage counts statements annotated PHP files execute into an aggregator outliving the runs.
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

// WithRouter shares router between the Route and Startup modules.
func WithRouter(router *stdruntime.Router) Option {
	return func(c *config) {
		c.router = router
	}
}
