package server

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/titpetric/cli"
	"github.com/titpetric/platform"

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

	root         string
	includeCache *runner.IncludeCache
	exprCache    *runner.ExprCache
}

// NewModule creates the HTTP module.
func NewModule(root string) *Module {
	return &Module{
		UnimplementedModule: *platform.NewUnimplementedModule("phpserver"),
		root:                root,
		includeCache:        runner.NewIncludeCache(),
		exprCache:           runner.NewExprCache(),
	}
}

// Mount registers HTTP routes.
func (m *Module) Mount(ctx context.Context, r platform.Router) error {
	r.Get("/*", m.handleRequest)
	return nil
}

// handleRequest resolves the request path and renders the file.
func (m *Module) handleRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	path := r.URL.Path
	if path == "/" {
		path = "/index.php"
	}

	filename := filepath.Join(m.root, filepath.Clean(path))

	result, headers, status, err := m.renderFile(ctx, filename, r)
	if err != nil {
		log.Printf("Error in request %s, %s: %s\n", path, filename, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Default content type, then apply any headers staged by PHP header().
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	for name, values := range headers {
		w.Header()[name] = values
	}
	if status != 0 {
		w.WriteHeader(status)
	}
	_, _ = w.Write([]byte(result))
}

// renderFile executes a .php file and returns the output plus any response
// headers staged by the PHP header() function. The *http.Request is exposed to
// the script via a runner.Context ($_GET/$_POST/$_PATH, getallheaders, header).
func (m *Module) renderFile(ctx context.Context, filename string, r *http.Request) (string, http.Header, int, error) {
	var out bytes.Buffer

	rt := runner.New(&out, runner.Options{
		SAPI:   "cgi-phpscript",
		RootFS: os.DirFS(m.root),
	})
	rt.SetIncludeCache(m.includeCache)
	rt.SetExprCache(m.exprCache)
	prog, err := rt.LoadFile(filename)
	if err != nil {
		return "", nil, 0, err
	}

	stdlib.Register(rt)
	stdlib.RegisterFS(rt, ".")

	reqCtx := runner.FromRequest(r)
	reqCtx.Register(rt)

	if err := rt.Run(prog); err != nil {
		if _, ok := runner.IsExit(err); ok {
			return out.String(), reqCtx.ResponseHeaders(), reqCtx.ResponseStatus(), nil
		}
		return "", nil, 0, err
	}

	return out.String(), reqCtx.ResponseHeaders(), reqCtx.ResponseStatus(), nil
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
