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

	routesvc "github.com/titpetric/phpscript/route"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// Name is the command title.
const Name = "Run php server"

// NewCommand creates a new server command.
func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "server",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args)
		},
	}
}

// Module implements the platform.Module interface.
type Module struct {
	platform.UnimplementedModule
	root string
}

// NewModule creates the HTTP module.
func NewModule(root string) *Module {
	return &Module{
		UnimplementedModule: *platform.NewUnimplementedModule("phpserver"),
		root:                root,
	}
}

// Mount registers HTTP routes.
func (m *Module) Mount(ctx context.Context, r platform.Router) error {
	handler, err := newHandler(os.DirFS(m.root), m.root)
	if err != nil {
		return err
	}
	r.Handle("/*", handler)
	return nil
}

type handler struct {
	root         fs.FS
	rootDir      string
	public       fs.FS
	static       http.Handler
	includeCache *runner.IncludeCache
	exprCache    *runner.ExprCache
}

// NewHandler serves annotated project routes and files beneath public/.
func NewHandler(root fs.FS) (http.Handler, error) {
	return newHandler(root, "")
}

func newHandler(root fs.FS, rootDir string) (http.Handler, error) {
	public, err := fs.Sub(root, "public")
	if err != nil {
		return nil, fmt.Errorf("server: public directory: %w", err)
	}
	h := &handler{
		root:         root,
		rootDir:      rootDir,
		public:       public,
		static:       http.FileServer(http.FS(public)),
		includeCache: runner.NewIncludeCache(),
		exprCache:    runner.NewExprCache(),
	}
	mux := http.NewServeMux()
	opts := []routesvc.Option{routesvc.WithExcludedDirectory("public")}
	if rootDir != "" {
		opts = append(opts, routesvc.WithRuntimeFunc(func(rt *runner.Runtime) {
			stdlib.RegisterFS(rt, rootDir)
		}))
	}
	if _, err := routesvc.NewService(root, mux, opts...); err != nil {
		return nil, err
	}
	mux.Handle("/", h)
	return mux, nil
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

	rt := runner.New(&out, runner.Options{
		SAPI:   "cgi-phpscript",
		RootFS: h.root,
	})
	rt.SetIncludeCache(h.includeCache)
	rt.SetExprCache(h.exprCache)
	rt.SetContext(r.Context())
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
func Run(ctx context.Context, args []string) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	opts := platform.NewOptions()
	opts.ServerAddr = ":8080"

	svc := platform.New(opts)
	svc.Register(NewModule(root))

	err := svc.Start(ctx)
	if err != nil {
		return err
	}

	// Wait until SIGINT/SIGTERM shutdown.
	svc.Wait()

	return nil
}
