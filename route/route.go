// Package route scans a PHP source tree of .php files and registers Go standard
// library routes for annotated PHP endpoints on a standard library mux router.
//
// Each PHP endpoint gives a hint using a comment with an annotation tag:
//
//   - `// @route GET /users/{id}`
//   - `// @route POST /users/{id}`
//   - `// @route <method> <path>`
//
// An annotation tag can be repeated to register multiple handlers. In the case
// of a duplicate handler being registered, the last one wins and a warning is
// printed in the logs. The annotation tag can be followed by an optional colon.
// One route per line.
//
// If method is omitted, only GET and POST are routed to the handler. This
// ignores requests like HEAD and OPTIONS, ideally leaving these to be resolved
// in the router, rather than invoking PHP.
//
// Specific HTTP methods like PUT are only reachable when explicitly stated.
//
// The .php files are scanned recursively, so you may keep them in a single
// folder, or just place them in arbitrary subfolders. Files without annotations
// will be skipped.
//
// This functionality fills a phpscript-specific auto-global value:
//
//   - `$_PATH`, specifically `$_PATH['id']`.
//
// It relies on the Go standard library to extract path parameters present.
package route

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/platform"
)

// RuntimeFunc customizes a PHP runtime before a routed PHP endpoint executes.
type RuntimeFunc func(*runner.Runtime)

// Option configures Service.
type Option func(*Service)

type routeRegistrar interface {
	Handle(string, string, http.Handler)
}

// WithRuntimeFunc registers fn to customize each request runtime.
func WithRuntimeFunc(fn RuntimeFunc) Option {
	return func(m *Service) {
		if fn != nil {
			m.runtimeFuncs = append(m.runtimeFuncs, fn)
		}
	}
}

// WithObservers attaches runtime observers to every routed request.
func WithObservers(observers ...runner.Observer) Option {
	return WithRuntimeFunc(func(runtime *runner.Runtime) {
		for _, observer := range observers {
			runtime.Observe(observer)
		}
	})
}

// WithRunnerOptions configures runtimes created for routed PHP endpoints.
func WithRunnerOptions(options runner.Options) Option {
	return func(m *Service) {
		m.runnerOptions = options
	}
}

// WithFlatstack enables the flat bytecode runtime with interpreter fallback.
func WithFlatstack(enabled bool) Option {
	return func(m *Service) {
		m.flatstack = enabled
	}
}

// Service owns route registration for annotated PHP endpoints.
type Service struct {
	mux           *http.ServeMux
	router        routeRegistrar
	warnings      []string
	runtimeFuncs  []RuntimeFunc
	exprCache     *runner.ExprCache
	excludedDirs  map[string]struct{}
	runnerOptions runner.Options
	flatstack     bool
}

// Module loads annotated PHP routes into a platform router.
type Module struct {
	platform.UnimplementedModule
	root    fs.FS
	options []Option
}

// NewModule creates an annotated route module.
func NewModule(root fs.FS, options ...Option) *Module {
	return &Module{
		UnimplementedModule: *platform.NewUnimplementedModule("phproute"),
		root:                root,
		options:             options,
	}
}

// Mount registers all discovered routes with the platform router.
func (m *Module) Mount(_ context.Context, router platform.Router) error {
	_, err := newService(m.root, platformRouteRegistrar{
		Router: router,
	}, m.options...)
	return err
}

// WithExcludedDirectory skips a top-level directory while scanning routes.
func WithExcludedDirectory(name string) Option {
	return func(m *Service) {
		name = strings.Trim(name, "/")
		if name != "" {
			m.excludedDirs[name] = struct{}{}
		}
	}
}

// WithExprCache sets a shared expression cache to the service.
func WithExprCache(cache *runner.ExprCache) Option {
	return func(m *Service) {
		m.exprCache = cache
	}
}

// NewService registers annotated PHP endpoints from root on mux.
func NewService(root fs.FS, mux *http.ServeMux, opts ...Option) (*Service, error) {
	if mux == nil {
		return nil, fmt.Errorf("route: nil mux")
	}
	service, err := newService(root, serveMuxRouteRegistrar{
		ServeMux: mux,
	}, opts...)
	if service != nil {
		service.mux = mux
	}
	return service, err
}

func newService(root fs.FS, router routeRegistrar, opts ...Option) (*Service, error) {
	svc := &Service{
		router:       router,
		excludedDirs: make(map[string]struct{}),
	}
	for _, opt := range opts {
		opt(svc)
	}
	if err := svc.Register(root); err != nil {
		return nil, err
	}
	if len(svc.warnings) > 0 {
		log.Println("Router loaded with warnings:")
		for _, warn := range svc.warnings {
			log.Println("WARN", warn)
		}
	}
	return svc, nil
}

// Register walks root for .php files and registers their @route annotations.
func (m *Service) Register(root fs.FS) error {
	if root == nil {
		return fmt.Errorf("route: nil root filesystem")
	}
	if m.exprCache == nil {
		m.exprCache = runner.NewExprCache()
	}
	// Include paths are relative to one filesystem root. Keep a cache per
	// Register call so two roots containing the same path cannot share a parsed
	// program accidentally.
	includeCache := runner.NewIncludeCache()
	seen := map[string]string{}
	return fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if _, excluded := m.excludedDirs[path]; excluded {
				return fs.SkipDir
			}
		}
		if d.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		b, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		for _, route := range Annotations(b) {
			pattern := route.Method + " " + route.Path
			if prev, ok := seen[pattern]; ok {
				m.warnings = append(m.warnings, fmt.Sprintf("duplicate route %q in %s; previously registered by %s", pattern, path, prev))
			}
			seen[pattern] = path
			file := path
			m.router.Handle(route.Method, route.Path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				m.servePHP(root, file, includeCache, w, r)
			}))
		}
		return nil
	})
}

type serveMuxRouteRegistrar struct {
	*http.ServeMux
}

func (r serveMuxRouteRegistrar) Handle(method string, routePath string, handler http.Handler) {
	if routePath == "/" {
		routePath = "/{$}"
	}
	r.ServeMux.Handle(method+" "+routePath, handler)
}

type platformRouteRegistrar struct {
	platform.Router
}

func (r platformRouteRegistrar) Handle(method string, routePath string, handler http.Handler) {
	r.Router.Method(method, routePath, handler)
}

// Annotations returns @route declarations from src. A path-only annotation
// expands to both GET and POST, matching the HTTP router.
func Annotations(src []byte) []model.RouteAnnotation {
	var routes []model.RouteAnnotation
	for _, line := range bytes.Split(src, []byte("\n")) {
		text := strings.TrimSpace(string(line))
		text = strings.TrimPrefix(text, "//")
		text = strings.TrimSpace(text)
		if !strings.HasPrefix(text, "@route") {
			continue
		}
		text = strings.TrimSpace(strings.TrimPrefix(text, "@route"))
		text = strings.TrimPrefix(text, ":")
		fields := strings.Fields(strings.TrimSpace(text))
		switch len(fields) {
		case 1:
			routes = append(routes, model.RouteAnnotation{Method: http.MethodGet, Path: fields[0]})
			routes = append(routes, model.RouteAnnotation{Method: http.MethodPost, Path: fields[0]})
		default:
			routes = append(routes, model.RouteAnnotation{Method: strings.ToUpper(fields[0]), Path: fields[1]})
		}
	}
	return routes
}

func (m *Service) servePHP(root fs.FS, file string, includeCache *runner.IncludeCache, w http.ResponseWriter, r *http.Request) {
	var out strings.Builder
	options := m.runnerOptions
	options.RootFS = root
	options.SAPI = "http"
	newRuntime := runner.New
	if m.flatstack {
		newRuntime = runner.NewFlatStack
	}
	rt := newRuntime(&out, options)
	rt.SetIncludeCache(includeCache)
	rt.SetExprCache(m.exprCache)
	rt.SetContext(r.Context())
	stdlib.Register(rt)
	ctx := runner.FromRequest(r)
	ctx.Register(rt)
	for _, fn := range m.runtimeFuncs {
		fn(rt)
	}
	prog, err := rt.LoadFile(file)
	if err == nil {
		err = rt.Run(prog)
	}
	for k, values := range ctx.ResponseHeaders() {
		w.Header()[k] = values
	}
	if err != nil {
		if _, ok := runner.IsExit(err); ok {
			if status := ctx.ResponseStatus(); status != 0 {
				w.WriteHeader(status)
			}
			_, _ = io.WriteString(w, out.String())
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if status := ctx.ResponseStatus(); status != 0 {
		w.WriteHeader(status)
	}
	_, _ = io.WriteString(w, out.String())
}

// WithContextValue returns middleware that stores key/value on request context.
func WithContextValue(key, value any, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), key, value)))
	})
}
