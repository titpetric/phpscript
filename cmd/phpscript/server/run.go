package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/titpetric/cli"
	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/config"
	routesvc "github.com/titpetric/phpscript/route"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/status"
)

// Name is the command title.
const Name = "Run php server"

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
	root          string
	runnerOptions runner.Options
	flatstack     bool
	observers     []runner.Observer
}

// NewModule creates the HTTP module.
func NewModule(root string, options runner.Options, flatstack bool, observers ...runner.Observer) *Module {
	return &Module{
		UnimplementedModule: *platform.NewUnimplementedModule("phpserver"),
		root:                root,
		runnerOptions:       options,
		flatstack:           flatstack,
		observers:           observers,
	}
}

// Mount registers HTTP routes.
func (m *Module) Mount(ctx context.Context, r platform.Router) error {
	handler, err := newHandler(os.DirFS(m.root), m.root, m.runnerOptions, m.flatstack, m.observers...)
	if err != nil {
		return err
	}
	r.Handle("/*", handler)
	return nil
}

type handler struct {
	root          fs.FS
	rootDir       string
	public        fs.FS
	static        http.Handler
	includeCache  *runner.IncludeCache
	exprCache     *runner.ExprCache
	runnerOptions runner.Options
	flatstack     bool
	observers     []runner.Observer
}

// NewHandler serves annotated project routes and files beneath public/.
func NewHandler(root fs.FS) (http.Handler, error) {
	var options runner.Options
	return newHandler(root, "", options, false)
}

func newHandler(root fs.FS, rootDir string, options runner.Options, flatstack bool, observers ...runner.Observer) (http.Handler, error) {
	public, err := fs.Sub(root, "public")
	if err != nil {
		return nil, fmt.Errorf("server: public directory: %w", err)
	}
	h := &handler{
		root:          root,
		rootDir:       rootDir,
		public:        public,
		static:        http.FileServer(http.FS(public)),
		includeCache:  runner.NewIncludeCache(),
		exprCache:     runner.NewExprCache(),
		runnerOptions: options,
		flatstack:     flatstack,
		observers:     observers,
	}
	return h, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if filename == "" || filename == "." {
		filename = "index.php"
	}
	info, err := fs.Stat(h.public, filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !info.IsDir() && path.Ext(filename) == ".php" {
		h.servePHP(w, r, path.Join("public", filename))
		return
	}
	h.static.ServeHTTP(w, r)
}

func (h *handler) servePHP(w http.ResponseWriter, r *http.Request, filename string) {
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
	prog, err := rt.LoadFile(filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stdlib.Register(rt)
	if h.rootDir != "" {
		stdlib.RegisterFS(rt, h.rootDir)
	}
	reqCtx := runner.FromRequest(r)
	reqCtx.Register(rt)

	err = rt.Run(prog)
	for name, values := range reqCtx.ResponseHeaders() {
		w.Header()[name] = values
	}
	if err != nil {
		if _, ok := runner.IsExit(err); !ok {
			log.Printf("Error in request %s, %s: %s", r.URL.Path, filename, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	if status := reqCtx.ResponseStatus(); status != 0 {
		w.WriteHeader(status)
	}
	_, _ = io.WriteString(w, out.String())
}

// Run starts the platform lifecycle and waits for shutdown.
func Run(ctx context.Context, args []string, config config.Config) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	opts := platform.NewOptions()
	opts.ServerAddr = ":8080"

	svc := platform.New(opts)
	var observers []runner.Observer
	if config.Status.Enabled {
		serverStatus := status.NewModule(config.Status.Options)
		svc.Use(serverStatus.Middleware)
		svc.Register(serverStatus)
		observers = append(observers, serverStatus)
	}
	if config.Routes.Enabled {
		routeOptions := []routesvc.Option{
			routesvc.WithExcludedDirectory("public"),
			routesvc.WithRunnerOptions(config.Runner),
			routesvc.WithFlatstack(config.Flatstack.Enabled),
			routesvc.WithObservers(observers...),
			routesvc.WithRuntimeFunc(func(rt *runner.Runtime) {
				stdlib.RegisterFS(rt, root)
			}),
		}
		svc.Register(routesvc.NewModule(os.DirFS(root), routeOptions...))
	}
	svc.Register(NewModule(root, config.Runner, config.Flatstack.Enabled, observers...))

	err := svc.Start(ctx)
	if err != nil {
		return err
	}

	// Wait until SIGINT/SIGTERM shutdown.
	svc.Wait()

	return nil
}
