package server

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	chi "github.com/go-chi/chi/v5"
	"github.com/titpetric/cli"
	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/database"
	"github.com/titpetric/phpscript/stdlib/files"
	"github.com/titpetric/phpscript/stdlib/smtp"
	"github.com/titpetric/phpscript/telemetry"
)

// Name is the command title.
const Name = "Run php server"

// DefaultDocumentRoot is the directory beneath an application root served over
// HTTP when the configuration names none.
const DefaultDocumentRoot = "public"

// NewCommand creates a new server command.
func NewCommand(config config.Config) *cli.Command {
	return &cli.Command{
		Name:  "server",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args, config)
		},
	}
}

// Module implements the platform.Module interface.
type Module struct {
	platform.UnimplementedModule
	handler http.Handler
}

// NewModule creates the HTTP module serving one application root.
func NewModule(root, documentRoot string, options runner.Options, flatstack, autoindex bool, observers ...runner.Observer) (*Module, error) {
	handler, err := newHandler(os.DirFS(root), root, documentRoot, options, flatstack, autoindex, observers...)
	if err != nil {
		return nil, err
	}
	return newModule("phpserver", handler), nil
}

// newModule wraps a handler as the platform module that mounts it on every
// path the router has not claimed.
func newModule(name string, handler http.Handler) *Module {
	return &Module{
		UnimplementedModule: *platform.NewUnimplementedModule(name),
		handler:             handler,
	}
}

// Mount registers HTTP routes.
func (m *Module) Mount(_ context.Context, r platform.Router) error {
	r.Handle("/*", m.handler)
	return nil
}

type handler struct {
	root          fs.FS
	rootDir       string
	documentRoot  string
	public        fs.FS
	includeCache  *runner.IncludeCache
	exprCache     *runner.ExprCache
	runnerOptions runner.Options
	flatstack     bool
	observers     []runner.Observer

	// autoindex answers a directory with no index page with a listing of
	// what is in it. See serveAutoindex.
	autoindex bool

	// smtp is the sender mail() delivers through, the site's own block for a
	// virtual host. The zero value keeps the stdlib default, a catchable
	// refusal naming the missing configuration.
	smtp smtp.Config

	// writable is where scripts may write, resolved against the application
	// root. Nothing in one of these directories is executed.
	writable []string
}

// NewHandler serves annotated project routes and files beneath public/.
func NewHandler(root fs.FS) (http.Handler, error) {
	var options runner.Options
	return newHandler(root, "", DefaultDocumentRoot, options, false, false)
}

func newHandler(root fs.FS, rootDir, documentRoot string, options runner.Options, flatstack, autoindex bool, observers ...runner.Observer) (*handler, error) {
	if documentRoot == "" {
		documentRoot = DefaultDocumentRoot
	}
	public, err := fs.Sub(root, documentRoot)
	if err != nil {
		return nil, fmt.Errorf("server: document root %q: %w", documentRoot, err)
	}
	h := &handler{
		root:          root,
		rootDir:       rootDir,
		documentRoot:  documentRoot,
		public:        public,
		includeCache:  runner.NewIncludeCache(),
		exprCache:     runner.NewExprCache(),
		runnerOptions: options,
		flatstack:     flatstack,
		autoindex:     autoindex,
		observers:     observers,
		writable:      files.WritableRoots(rootDir, options.WritablePaths),
	}
	return h, nil
}

// executes reports whether a .php entrypoint under the document root may run.
//
// A writable path holds what scripts, and through them their users, put there.
// A .php file that arrives that way is content, not code: it is served as bytes
// like anything else in the directory. An upload directory below the document
// root is the ordinary case, and executing what lands in one turns an upload
// form into a way to run code.
//
// filename is relative to the application root.
func (h *handler) executes(filename string) bool {
	if len(h.writable) == 0 {
		return true
	}
	name := filepath.Join(h.rootDir, filepath.FromSlash(filename))
	for _, dir := range h.writable {
		if files.Within(name, dir) {
			return false
		}
	}
	return true
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if filename == "" {
		filename = "."
	}
	info, err := fs.Stat(h.public, filename)
	if err != nil {
		h.serveNotFound(w, r)
		return
	}

	// A directory is answered by its index page. One with no index page is a
	// 404 rather than a listing of the files below it, unless the site turned
	// autoindex on and asked for exactly that.
	if info.IsDir() {
		// A directory named without its trailing slash is redirected first, so
		// the relative links in the page that answers resolve below it and not
		// beside it.
		if !strings.HasSuffix(r.URL.Path, "/") {
			redirectToDirectory(w, r)
			return
		}
		index, ok := h.indexPage(filename)
		if !ok {
			if h.serveAutoindex(w, r, filename) {
				return
			}
			h.serveNotFound(w, r)
			return
		}
		filename = index
	}

	if path.Ext(filename) == ".php" {
		// The entrypoint is named relative to the application root, not the
		// document root, so an include reaching above the served directory
		// still resolves inside the project.
		entrypoint := path.Join(h.documentRoot, filename)
		if h.executes(entrypoint) {
			h.servePHP(w, r, entrypoint)
			return
		}
		// A .php file in a writable directory is served, never run.
	}

	// The name is served rather than the URL path, because a directory's index
	// page is a file the request did not name.
	http.ServeFileFS(w, r, h.public, filename)
}

// serveNotFound answers a request that resolved to nothing: the site's own page
// when it has one, and the plain status otherwise. See serveErrorPage for who
// gets the page.
func (h *handler) serveNotFound(w http.ResponseWriter, r *http.Request) {
	if h.serveUnrouted(w, r, http.StatusNotFound) {
		return
	}
	http.NotFound(w, r)
}

// redirectToDirectory sends a request that named a directory without its
// trailing slash to the same path carrying one, query string and all.
//
// The location is relative, as the one http.FileServer writes is: an absolute
// path would be wrong under a mount that stripped a prefix off the request.
func redirectToDirectory(w http.ResponseWriter, r *http.Request) {
	target := path.Base(r.URL.Path) + "/"
	if query := r.URL.RawQuery; query != "" {
		target += "?" + query
	}
	w.Header().Set("Location", target)
	w.WriteHeader(http.StatusMovedPermanently)
}

// serverVars adds the $_SERVER entries only the server knows: where the site
// lives on disk and which script the request resolved to. runner.Context fills
// everything derivable from the request itself and deliberately leaves these
// out, because it has no document root and no resolved script.
//
// SERVER_NAME and SERVER_PORT come from the address the listener is bound to in
// real PHP, not from the Host header, which is client supplied. This server
// routes by Host, so the name a request arrived under is the site it reached,
// and that is what SERVER_NAME reports; the port is the one the URL carried,
// left unset when it carried none rather than guessed at.
//
// filename is relative to the application root, as LoadFile takes it.
func (h *handler) serverVars(request runner.Context, r *http.Request, filename string) {
	request.Server["DOCUMENT_ROOT"] = path.Join(h.rootDir, h.documentRoot)
	request.Server["SERVER_SOFTWARE"] = "phpscript"

	// The script path as a URL, which is the entrypoint with the document
	// root taken off the front: public/index.php is /index.php to a script.
	script := "/" + strings.TrimPrefix(filename, h.documentRoot+"/")
	request.Server["SCRIPT_NAME"] = script
	request.Server["PHP_SELF"] = script
	if h.rootDir != "" {
		request.Server["SCRIPT_FILENAME"] = filepath.Join(h.rootDir, filepath.FromSlash(filename))
	}

	host := r.Host
	if name, port, err := net.SplitHostPort(host); err == nil {
		host = name
		request.Server["SERVER_PORT"] = port
	}
	request.Server["SERVER_NAME"] = strings.TrimSuffix(strings.ToLower(host), ".")
}

// run executes one PHP entrypoint of this site and returns the response it
// produced: the request context holding the headers and status the script
// staged, the body it buffered, and whatever it ended with.
//
// Nothing reaches w. The caller is what decides the response, which is what
// lets an error be answered with the site's own page rather than with the
// error's own words. vars, when set, adds $_SERVER entries the entrypoint needs
// beyond the ones the request and the site answer for.
func (h *handler) run(w http.ResponseWriter, r *http.Request, filename string, vars func(runner.Context)) (runner.Context, []byte, error) {
	var out bytes.Buffer

	options := h.runnerOptions
	options.SAPI = "cgi-phpscript"
	options.RootFS = h.root
	newRuntime := runner.New
	if h.flatstack {
		newRuntime = runner.NewFlatStack
	}
	rt := newRuntime(&out, options)
	rt.SetIncludeCache(h.includeCache)
	rt.SetExprCache(h.exprCache)
	rt.SetContext(r.Context())
	for _, observer := range h.observers {
		rt.Observe(observer)
	}

	stdlib.Register(rt)
	if h.rootDir != "" {
		stdlib.RegisterFS(rt, h.rootDir)
	}
	smtp.RegisterConfig(rt, h.smtp)
	reqCtx := runner.FromRequestOptions(r, options)
	// Uploaded parts are copied to temporary files for the script to read; they
	// belong to this request and nothing outlives it.
	defer reqCtx.Cleanup()
	h.serverVars(reqCtx, r, filename)
	if vars != nil {
		vars(reqCtx)
	}
	reqCtx.Register(rt)
	// The parsed request and the response writer live for this request too;
	// Register accounted the Context, these are the host structures around it.
	rt.AccountRequest(r, w)

	prog, err := rt.LoadFile(filename)
	if err == nil {
		err = rt.Run(prog)
	}

	if trace := telemetry.TraceFromContext(r.Context()); trace != nil {
		// The script's frames are gone once Run returns, so the peak is the
		// request's memory footprint; a fresh usage walk would be baseline.
		trace.Root().SetAttribute("memory_usage", rt.MemoryPeak())
		if rt.MemoryLimit() > 0 {
			trace.Root().SetAttribute("memory_limit", rt.MemoryLimit().Bytes())
		}
	}
	return reqCtx, out.Bytes(), err
}

func (h *handler) servePHP(w http.ResponseWriter, r *http.Request, filename string) {
	reqCtx, body, err := h.run(w, r, filename, nil)
	status := reqCtx.StatusFor(err)

	// What went wrong, for the site's error page to do as it likes with. It is
	// set only for a failure, and a failure discards whatever the script echoed
	// before it: half a page is not an answer.
	var notes string
	if _, exited := runner.IsExit(err); err != nil && !exited {
		// The trace ID is also the Request-Id header, so a log line and the
		// recorded trace of the same request find each other.
		log.Printf("Error in request %s, %s: %s [trace %s]", r.URL.Path, filename, err, telemetry.TraceID(r.Context()))
		notes, body = err.Error(), nil
	}

	if status >= 400 && !reqCtx.Answered(body) && h.serveErrorPage(w, r, status, notes) {
		return
	}
	if notes != "" {
		serveStatus(w, status)
		return
	}
	reqCtx.SetDefaultHeader("Content-Type", "text/html; charset=utf-8")
	reqCtx.WriteResponse(w, status, body)
}

// serveStatus answers with the status and its standard text, and no more.
//
// What actually went wrong is in the log line and on the request trace, both
// addressed by the same request id. It used to be in the response body as well,
// where it is a description of the site's own internals handed to whoever
// asked for it.
func serveStatus(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

// Run starts the platform lifecycle and waits for shutdown.
func Run(ctx context.Context, args []string, appConfig config.Config) error {
	options, err := appConfig.PlatformOptions()
	if err != nil {
		return err
	}

	svc := platform.New(options)

	// The platform records, phpscript observes. Its recorder owns the tracer
	// and its middleware is what puts a trace in the request context, so the
	// interpreter reports onto that trace instead of into a second recorder of
	// its own. Telemetry turned off leaves no recorder to find, and the
	// observers stay empty.
	var observers []runner.Observer
	var recorder *platform.TelemetryModule
	if svc.Find(&recorder) {
		observers = append(observers, telemetry.NewModule(recorder.Tracer()))
	}

	if len(appConfig.VirtualHost) > 0 {
		if len(args) > 0 {
			return fmt.Errorf("server: the configuration lists virtual hosts, which name their own roots; %q on the command line has no virtual host to belong to", args[0])
		}
		if err := registerVirtualHosts(ctx, svc, appConfig); err != nil {
			return err
		}
	} else {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		if err := registerSite(svc, appConfig, observers, root); err != nil {
			return err
		}
	}

	if err := svc.Start(ctx); err != nil {
		return err
	}

	// Wait until SIGINT/SIGTERM shutdown.
	svc.Wait()

	return nil
}

// registerSite wires the single tenant server: one application root, its
// modules mounted straight onto the platform router. The route modules stay on
// that router rather than a nested one so the platform keeps seeing the pattern
// a request matched, which is what its traces are labelled with.
func registerSite(svc *platform.Platform, appConfig config.Config, observers []runner.Observer, root string) error {
	documentRoot := documentRoot(appConfig)

	// One application owns the process, so its scripts read the process
	// environment with what the configuration adds on top. Infrastructure
	// variables are held back by runner.ScriptEnvironment either way.
	runnerOptions := appConfig.Runner
	runnerOptions.Env = append(append([]string{}, os.Environ()...), appConfig.Env...)

	// Startup jobs and routed endpoints read the same source tree and execute
	// PHP the same way, so they share one set of options.
	annotationOptions := annotationOptions(appConfig, runnerOptions, observers, root, "")

	// A single application server keeps a failing @startup fatal: there is no
	// other tenant to protect, and a process that came up with its schema
	// unapplied is worse than one that did not come up.
	svc.Register(annotations.NewStartup(os.DirFS(root), annotationOptions...))
	svc.Register(annotations.NewScheduler(os.DirFS(root), annotationOptions...))

	files, err := newHandler(os.DirFS(root), root, documentRoot, runnerOptions, appConfig.Flatstack.Enabled, appConfig.Autoindex, observers...)
	if err != nil {
		return err
	}
	files.smtp = appConfig.SMTP
	// The routed endpoints are handed the file handler's error pages: they live
	// under the site's document root, which is the file handler's to look in.
	if appConfig.Routes.Enabled {
		options := routeOptions(annotationOptions, runnerOptions.WritablePaths, documentRoot)
		options = append(options, annotations.WithErrorPages(files.serveErrorPage))
		svc.Register(annotations.NewRoute(os.DirFS(root), options...))
	}
	svc.Register(newModule("phpserver", files))
	return nil
}

// registerVirtualHosts wires one site per configured domain and puts a host mux
// in front of them.
func registerVirtualHosts(ctx context.Context, svc *platform.Platform, appConfig config.Config) error {
	handler, modules, err := buildVirtualHosts(ctx, appConfig)
	if err != nil {
		return err
	}
	for _, module := range modules {
		svc.Register(module)
	}
	svc.Register(newModule("phpvhost", handler))
	return nil
}

// buildVirtualHosts returns the host mux serving every configured site and the
// lifecycle modules the platform starts on their behalf. Every site is built
// here, before the server starts, so a broken configuration fails startup
// rather than one request.
func buildVirtualHosts(ctx context.Context, appConfig config.Config) (http.Handler, []platform.Module, error) {
	if err := appConfig.ValidateVirtualHosts(); err != nil {
		return nil, nil, err
	}

	var modules []platform.Module
	sites := make(map[string]http.Handler, len(appConfig.VirtualHost))
	for _, host := range appConfig.VirtualHost {
		host = host.Normalize()

		siteConfig, err := host.Load(appConfig)
		if err != nil {
			return nil, nil, err
		}

		handler, siteModules, err := newVirtualHost(ctx, host, siteConfig)
		if err != nil {
			return nil, nil, err
		}
		modules = append(modules, siteModules...)

		// Every name the entry lists reaches the same handler. A further
		// name is another way to spell the site, not another copy of it,
		// so it shares the site's routes, recorder and connections.
		for _, domain := range host.Domains() {
			sites[domain] = handler
		}
	}

	return newHostMux(sites), modules, nil
}

// newVirtualHost builds the router one site is served by and the lifecycle
// modules the platform starts on its behalf.
//
// The site gets a router of its own because it owns its routes, its telemetry
// and its databases. Nothing it registers is visible on another domain, and the
// connections its scripts can name are only the ones its own env configured.
func newVirtualHost(ctx context.Context, host config.VirtualHost, siteConfig config.Config) (http.Handler, []platform.Module, error) {
	router := chi.NewRouter()

	// A site answering to several names is still one site, reported and named
	// after the first of them.
	name := host.Name()

	// The site owns its telemetry block, so it owns the tracer and the debug
	// front end that block describes. chi wants every middleware before the
	// first route, so this comes first.
	var observers []runner.Observer
	telemetryOptions, err := siteConfig.Telemetry.Resolved()
	if err != nil {
		return nil, nil, fmt.Errorf("virtualhost %q: %w", name, err)
	}
	if telemetryOptions.Enabled {
		tracer, err := telemetry.New(telemetryOptions)
		if err != nil {
			return nil, nil, fmt.Errorf("virtualhost %q: telemetry: %w", name, err)
		}
		telemetryOptions.Tracer = tracer
		router.Use(telemetry.TracingMiddleware(telemetryOptions))
		if err := telemetry.Mount(router, telemetryOptions); err != nil {
			return nil, nil, fmt.Errorf("virtualhost %q: telemetry: %w", name, err)
		}
		observers = append(observers, telemetry.NewModule(tracer))
	}

	// The site's connections come from its own env and nowhere else. A
	// provider holds the credentials it was given, so a site cannot name a
	// connection another site configured.
	runnerOptions := siteConfig.Runner
	runnerOptions.Database = database.New(siteConfig.Env)

	// So does the environment its scripts read. A site is handed the env it
	// declared rather than the process environment, so getenv() cannot be
	// used to read what the operator, or another site, was started with.
	runnerOptions.Env = siteConfig.Env

	documentRoot := documentRoot(siteConfig)
	annotationOptions := annotationOptions(siteConfig, runnerOptions, observers, host.Root, name)

	root := os.DirFS(host.Root)
	files, err := newHandler(root, host.Root, documentRoot, runnerOptions, siteConfig.Flatstack.Enabled, siteConfig.Autoindex, observers...)
	if err != nil {
		return nil, nil, fmt.Errorf("virtualhost %q: %w", name, err)
	}
	files.smtp = siteConfig.SMTP

	// A site's error pages are its own: they are found under its document root
	// and rendered by its own handler, so nothing a site puts up is reachable
	// from another domain.
	if siteConfig.Routes.Enabled {
		options := routeOptions(annotationOptions, runnerOptions.WritablePaths, documentRoot)
		options = append(options, annotations.WithErrorPages(files.serveErrorPage))
		routes := annotations.NewRoute(root, options...)
		if err := routes.Mount(ctx, router); err != nil {
			return nil, nil, fmt.Errorf("virtualhost %q: %w", name, err)
		}
	}

	router.Handle("/*", files)

	// A site's startup failure is its own. It is recorded on that site's
	// recorder and the server carries on, because the alternative is one
	// tenant's broken job stopping every other tenant.
	return router, []platform.Module{
		nonFatal(annotations.NewStartup(root, annotationOptions...), observers),
		annotations.NewScheduler(root, annotationOptions...),
	}, nil
}

// documentRoot returns the directory beneath the application root served over
// HTTP. public is the default and config.yml carries it; the literal here only
// covers a Config assembled in Go without one.
func documentRoot(appConfig config.Config) string {
	if appConfig.DocumentRoot == "" {
		return DefaultDocumentRoot
	}
	return appConfig.DocumentRoot
}

// annotationOptions returns the options every annotated PHP file of one source
// tree runs under. Startup jobs, scheduled jobs and routed endpoints read the
// same tree and execute PHP the same way, so they share one set.
func annotationOptions(appConfig config.Config, runnerOptions runner.Options, observers []runner.Observer, root, suffix string) []annotations.Option {
	return []annotations.Option{
		annotations.WithRootDir(root),
		annotations.WithOutput(os.Stdout),
		annotations.WithRunnerOptions(runnerOptions),
		annotations.WithFlatstack(appConfig.Flatstack.Enabled),
		annotations.WithObservers(observers...),
		annotations.WithModuleSuffix(suffix),
		// mail() delivers through the site's smtp block; with none configured
		// the stdlib default, a catchable refusal, is re-registered unchanged.
		annotations.WithRuntimeFunc(func(rt *runner.Runtime) {
			smtp.RegisterConfig(rt, appConfig.SMTP)
		}),
	}
}

// routeOptions adds the exclusions routed endpoints need.
//
// Files under the document root are served directly, so an annotation there
// must not publish a second, unguarded route. A writable path is excluded for a
// stronger reason: a script can put a .php file in one, so scanning it would let
// whatever a user uploaded register a route the next time the server starts.
func routeOptions(options []annotations.Option, writablePaths []string, documentRoot string) []annotations.Option {
	result := append(options[:len(options):len(options)], annotations.WithExcludedDirectory(documentRoot))
	for _, p := range writablePaths {
		// The scanner walks paths relative to the application root, so an
		// absolute entry names nothing it will ever see.
		if p = strings.TrimSpace(p); p != "" && !filepath.IsAbs(p) {
			result = append(result, annotations.WithExcludedDirectory(p))
		}
	}
	return result
}
