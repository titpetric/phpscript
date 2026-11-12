package http

import (
	"fmt"
	"io/fs"
	nethttp "net/http"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

func init() {
	runner.RegisterArea(`Runtime\PS\HTTP\Files`, registerFiles)
}

// registerFiles installs net/http's three file-serving entry points.
func registerFiles(rt *runner.Runtime) {
	// HTTP\ServeFile answers $r with the file at $path, writing the content type, the byte range and the conditional headers net/http writes for one file.
	rt.RegisterFunc("HTTP\\ServeFile", func(w, r any, path string) error {
		writer, request, err := exchange("HTTP\\ServeFile", w, r)
		if err != nil {
			return err
		}
		tree, name, err := rooted(rt, "HTTP\\ServeFile", path)
		if err != nil {
			return err
		}
		nethttp.ServeFileFS(writer, request, tree, name)
		return nil
	})

	// HTTP\FileServer is the handler that answers with the file under $dir the request path names, which $mux->handle takes in place of a callable.
	rt.RegisterConstructor("HTTP\\FileServer", func(dir string) (nethttp.Handler, error) {
		tree, name, err := rooted(rt, "HTTP\\FileServer", dir)
		if err != nil {
			return nil, err
		}
		under, err := fs.Sub(tree, name)
		if err != nil {
			return nil, fmt.Errorf("HTTP\\FileServer: %w", err)
		}
		return nethttp.FileServerFS(under), nil
	})

	// HTTP\StripPrefix answers through $handler with $prefix cut off the request path, which a file server registered on a subtree needs.
	rt.RegisterFunc("HTTP\\StripPrefix", func(prefix string, handler any) (nethttp.Handler, error) {
		given, ok := handler.(nethttp.Handler)
		if !ok {
			return nil, fmt.Errorf("HTTP\\StripPrefix: $handler is %s, and takes a handler such as HTTP\\FileServer", phpType(handler))
		}
		return nethttp.StripPrefix(prefix, given), nil
	})
}

// exchange reads back the two net/http values the mux called a handler with.
func exchange(name string, w, r any) (nethttp.ResponseWriter, *nethttp.Request, error) {
	writer, ok := w.(nethttp.ResponseWriter)
	if !ok {
		return nil, nil, fmt.Errorf("%s: $w is %s, and takes the response writer the handler was called with", name, phpType(w))
	}
	request, ok := r.(*nethttp.Request)
	if !ok {
		return nil, nil, fmt.Errorf("%s: $r is %s, and takes the request the handler was called with", name, phpType(r))
	}
	return writer, request, nil
}

// rooted resolves a path against the source filesystem, which no name climbs out of; docs/security.md argues the boundary.
func rooted(rt *runner.Runtime, name, path string) (fs.FS, string, error) {
	tree := rt.FS()
	if tree == nil {
		return nil, "", fmt.Errorf("%s: this runtime reads no source filesystem", name)
	}
	if strings.TrimSpace(path) == "" {
		return nil, "", fmt.Errorf("%s: a path is required", name)
	}
	return tree, rt.ResolvePath(path), nil
}

// phpType names a value for a refusal, in the spelling get_debug_type() uses.
func phpType(v any) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprintf("%T", v)
}
