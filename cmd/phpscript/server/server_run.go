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
	"slices"
	"strings"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/stdlib/ps/files"
	"github.com/titpetric/phpscript/stdlib/ps/mail"
	stdruntime "github.com/titpetric/phpscript/stdlib/ps/runtime"
	"github.com/titpetric/phpscript/telemetry"
)

// Name is the command title.
const Name = "Run php server"

// DefaultDocumentRoot is the directory beneath an application root served over HTTP.
const DefaultDocumentRoot = "public"

// NewCommand creates a new server command.
func NewCommand(cfg *config.Config, globals *flags.Options) *cli.Command {
	appConfig := config.Value(cfg)

	return &cli.Command{
		Name:  "server",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args, appConfig, globals)
		},
	}
}

// Module implements the LifecycleModule interface.
type Module struct {
	ZeroModule
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

// newModule wraps a handler as a module mounting it on unclaimed paths.
func newModule(name string, handler http.Handler) *Module {
	return &Module{
		ZeroModule: *newZeroModule(name),
		handler:    handler,
	}
}

// Mount registers HTTP routes.
func (m *Module) Mount(_ context.Context, r Router) error {
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

	// phpOptions holds runnerOptions for an entrypoint: the SAPI named, the
	// source root bound. The handler derives it once. The pool's runtimes and
	// request Contexts read the same value.
	phpOptions runner.Options

	// pool holds this site's registered runtimes between requests, a handler
	// field like the include cache. A runtime registered against this
	// site's filesystem and options can never answer for another site; see
	// runner.SessionPool.
	pool *runner.SessionPool

	// coverage counts the statements every request runs, folded per request so
	// no AST-keyed state outlives the program.
	// Nil is off, the state of a server started without --cover.
	coverage *coverage.Aggregator

	// autoindex answers a directory lacking an index page with a listing. See
	// serveAutoindex.
	autoindex bool

	// writable lists where scripts may write, resolved against the application
	// root. The handler never runs a file in one of these directories.
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
	h.phpOptions = options
	h.phpOptions.SAPI = "cgi-phpscript"
	h.phpOptions.RootFS = root
	h.pool = runner.NewSessionPool(0, h.buildRuntime)
	return h, nil
}

// executes reports whether a .php entrypoint may run, false in a writable root.
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

	// The index page answers a directory. A directory lacking an index page
	// gets a 404. The files below stay unlisted, unless the site enabled
	// autoindex for exactly that directory.
	if info.IsDir() {
		// A directory named without a trailing slash takes a redirect first.
		// In the answering page, relative links resolve below the directory;
		// none resolve beside it.
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
		// The entrypoint name is relative to the application root, not the
		// document root and an include reaching above the served directory
		// still resolves inside the project.
		entrypoint := path.Join(h.documentRoot, filename)
		if h.executes(entrypoint) {
			h.servePHP(w, r, entrypoint)
			return
		}
		// A .php file in a writable directory goes out as bytes, never runs.
	}

	// ServeFileFS takes the file name, ignoring the URL path. A directory's
	// index page is a file the request did not name.
	http.ServeFileFS(w, r, h.public, filename)
}

// serveNotFound answers with the site's error page or the plain 404 status.
func (h *handler) serveNotFound(w http.ResponseWriter, r *http.Request) {
	if h.serveUnrouted(w, r, http.StatusNotFound) {
		return
	}
	http.NotFound(w, r)
}

// redirectToDirectory sends a relative redirect adding the trailing slash, query string included.
func redirectToDirectory(w http.ResponseWriter, r *http.Request) {
	target := path.Base(r.URL.Path) + "/"
	if query := r.URL.RawQuery; query != "" {
		target += "?" + query
	}
	w.Header().Set("Location", target)
	w.WriteHeader(http.StatusMovedPermanently)
}

// serverVars adds the $_SERVER entries only the server knows, filename app-relative.
func (h *handler) serverVars(request runner.Context, r *http.Request, filename string) {
	request.Server["DOCUMENT_ROOT"] = path.Join(h.rootDir, h.documentRoot)
	request.Server["SERVER_SOFTWARE"] = "phpscript"

	// The script path as a URL: the entrypoint with the document
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

// run executes one entrypoint, returning the staged context, buffered body and error.
func (h *handler) run(w http.ResponseWriter, r *http.Request, filename string, vars func(runner.Context)) (runner.Context, []byte, error) {
	var out bytes.Buffer

	// A pooled runtime, registered once by buildRuntime and reset between
	// requests: the Put drops everything a request wrote. The response is a
	// pushed redirection, dropped with the rest of the session by the reset.
	// The connection enters through BeginRequest, as a Path B worker's does.
	// connection_aborted answers for this client; ignore_user_abort starts off
	// for every request.
	rt := h.pool.Get()
	defer h.pool.Put(rt)
	rt.PushOutput(&out)
	defer rt.EndRequest(rt.BeginRequest(r.Context()))
	rt.SetContext(r.Context())
	if h.coverage != nil {
		// A collector turns flatstack off for this request: coverage is an
		// interpreter feature and the fallback is atomic. The pool drops the
		// collector with the session; nothing keyed by an AST node outlives the
		// program.
		collector := coverage.New()
		rt.SetCoverage(collector)
		defer h.coverage.Add(collector)
	}

	reqCtx := runner.FromRequestOptions(r, h.phpOptions)
	// Uploaded parts land in temporary files for the script to read; they
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
		// No script frame outlives Run and the peak is the request's memory
		// footprint; a fresh usage walk would be baseline.
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

	// What went wrong, for the site's error page to use. A failure sets the
	// value. A failure discards whatever the script echoed before it: half a page
	// is not an answer.
	var notes string
	if _, exited := runner.IsExit(err); err != nil && !exited {
		// The trace ID appears as the Request-Id header, in the log line, and
		// in the recorded trace of the same request.
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

// serveStatus answers the status and its standard text, holding nothing of the error.
func serveStatus(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

// Run starts the platform lifecycle and waits for shutdown.
func Run(ctx context.Context, args []string, appConfig config.Config, globals *flags.Options) error {
	manager, err := newManager(ctx, args, appConfig, globals)
	if err != nil {
		return err
	}

	if err := manager.Start(ctx); err != nil {
		return err
	}

	// Until SIGINT or SIGTERM. A SIGHUP reloads and does not end this.
	manager.Wait()

	return nil
}

// newManager builds the manager Run starts, with nothing bound yet.
func newManager(ctx context.Context, args []string, started config.Config, globals *flags.Options) (*Manager, error) {
	options, err := started.PlatformOptions()
	if err != nil {
		return nil, err
	}

	manager := newPlatformManager(options)

	// One aggregator for the process: a reload replaces the sites and keeps
	// the measurement of this process's work.
	var cover *coverageModule
	if globals.Covering() {
		cover = newCoverageModule(globals)
	}

	// Everything -t runs, before the swap: a later failure arrives with the
	// old generation stopped, and the manager then exits. Loading the file is
	// not enough, a virtual host lacking a root parses.
	name, root := configName(globals.ConfigFile), rootArg(args)
	manager.Check = func() error {
		appConfig, err := reloadConfig(started, globals)
		if err != nil {
			return err
		}
		return appConfig.Check(name, root)
	}

	// A reload discards the platform value every registration binds to. Each
	// registration belongs here, not in Run.
	manager.Setup = func(svc *Service) error {
		appConfig, err := reloadConfig(started, globals)
		if err != nil {
			return err
		}
		warnFrozen(svc.Logger, started, appConfig)
		return setup(ctx, svc, appConfig, args, globals, cover)
	}

	return manager, nil
}

// reloadConfig reads the configuration from disk again, returning started without -f.
func reloadConfig(started config.Config, globals *flags.Options) (config.Config, error) {
	if globals.ConfigFile == "" {
		return started, nil
	}
	return config.Load(globals.ConfigFile)
}

// setup registers each generation's services.
func setup(ctx context.Context, svc *Service, appConfig config.Config, args []string, globals *flags.Options, cover *coverageModule) error {
	// The platform records, phpscript observes: the interpreter reports onto
	// the trace the recorder's middleware put in the request context, never
	// into a second recorder. Looked up per generation; each builds a separate
	// recorder. Telemetry off leaves the observers empty.
	var observers []runner.Observer
	var recorder *TelemetryModule
	if svc.Find(&recorder) {
		observers = append(observers, telemetry.NewModule(recorder.Tracer()))
	}

	if len(appConfig.VirtualHost) > 0 {
		if len(args) > 0 {
			return config.ErrRootWithVirtualHosts(args[0])
		}
		if err := registerVirtualHosts(ctx, svc, appConfig, globals, cover); err != nil {
			return err
		}
	} else {
		root := rootArg(args)
		if root == "" {
			root = "."
		}
		if err := registerSite(svc, appConfig, observers, root, globals, cover); err != nil {
			return err
		}
	}

	// The profile goes last and stops last: the platform stops modules in
	// registration order. A profile written before the sites stopped would
	// miss what they did on the way out.
	if cover != nil {
		svc.Register(cover)
	}
	return nil
}

// warnFrozen reports a reloaded setting the manager cannot apply without a restart.
func warnFrozen(log Logger, started, reloaded config.Config) {
	if log == nil {
		return
	}

	var changed []string
	if started.Server.Addr != reloaded.Server.Addr {
		changed = append(changed, "server.addr")
	}
	if started.Server.Quiet != reloaded.Server.Quiet {
		changed = append(changed, "server.quiet")
	}
	if !slices.Equal(started.Server.Modules, reloaded.Server.Modules) {
		changed = append(changed, "server.modules")
	}
	if started.Server.PidFile != reloaded.Server.PidFile {
		changed = append(changed, "server.pid_file")
	}

	if len(changed) > 0 {
		log.Info("reloaded settings that need a restart", "keys", changed)
	}
}

// registerSite wires the single-tenant server, mounting modules onto the platform router.
func registerSite(svc *Service, appConfig config.Config, observers []runner.Observer, root string, globals *flags.Options, cover *coverageModule) error {
	documentRoot := documentRoot(appConfig)

	// One application owns the process and its scripts read the process
	// environment plus whatever the configuration adds. Either way,
	// runner.ScriptEnvironment holds infrastructure variables back.
	runnerOptions := appConfig.Runner
	runnerOptions.Env = append(append([]string{}, os.Environ()...), appConfig.Env...)

	// The mail servers its scripts can name are the ones the configuration
	// named, and the credentials behind them stay with the provider.
	runnerOptions.Mail = mail.NewProvider(appConfig.Mail)

	if globals.Include != "" {
		runnerOptions.Include = globals.Include
	}

	// newHandler runs first because the handler owns the site's caches, and
	// the annotated files run off the same pair.
	files, err := newHandler(os.DirFS(root), root, documentRoot, runnerOptions, appConfig.Flatstack.Enabled, appConfig.Autoindex, observers...)
	if err != nil {
		return err
	}

	// Startup jobs and routed endpoints read the same source tree and execute
	// PHP the same way and they share one set of options.
	annotationOptions := sharedCaches(files, annotationOptions(appConfig, runnerOptions, observers, root, ""))
	if cover != nil {
		cover.watch(os.DirFS(root))
		annotationOptions = append(annotationOptions, annotations.WithCoverage(cover.aggregator))
		files.coverage = cover.aggregator
	}

	// A single application server keeps a failing @startup fatal. No other
	// tenant needs protection. A process that came up with its schema
	// unapplied is worse than one still down.
	svc.Register(annotations.NewStartup(os.DirFS(root), annotationOptions...))
	svc.Register(annotations.NewScheduler(os.DirFS(root), annotationOptions...))

	// A @startup job's bindings go on the router even where the tree holds no
	// @route annotations. The registration lives with the jobs, not with the
	// scanner: a service binds where a script annotates.
	svc.Register(annotations.NewBindings(annotationOptions...))

	// The routed endpoints take the file handler's error pages, held in the
	// site's document root, the directory the file handler looks in.
	if appConfig.Routes.Enabled {
		options := routeOptions(annotationOptions, runnerOptions.WritablePaths, documentRoot)
		options = append(options, annotations.WithErrorPages(files.serveErrorPage))
		svc.Register(annotations.NewRoute(os.DirFS(root), options...))
	}
	precompile(files, root)
	svc.Register(newModule("phpserver", files))
	return nil
}

// documentRoot returns the directory served over HTTP, defaulting to public.
func documentRoot(appConfig config.Config) string {
	if appConfig.DocumentRoot == "" {
		return DefaultDocumentRoot
	}
	return appConfig.DocumentRoot
}

// annotationOptions returns the options every annotated file of one source tree runs under.
func annotationOptions(appConfig config.Config, runnerOptions runner.Options, observers []runner.Observer, root, suffix string) []annotations.Option {
	return []annotations.Option{
		annotations.WithRootDir(root),
		annotations.WithOutput(os.Stdout),
		annotations.WithRunnerOptions(runnerOptions),
		annotations.WithFlatstack(appConfig.Flatstack.Enabled),
		annotations.WithObservers(observers...),
		annotations.WithModuleSuffix(suffix),
		annotations.WithRouter(stdruntime.NewRouter()),
	}
}

// routeOptions excludes the document root and writable paths from annotation scans.
func routeOptions(options []annotations.Option, writablePaths []string, documentRoot string) []annotations.Option {
	result := append(options[:len(options):len(options)], annotations.WithExcludedDirectory(documentRoot))
	for _, p := range writablePaths {
		// The scanner walks paths relative to the application root and an
		// absolute entry names nothing it will ever see.
		if p = strings.TrimSpace(p); p != "" && !filepath.IsAbs(p) {
			result = append(result, annotations.WithExcludedDirectory(p))
		}
	}
	return result
}
