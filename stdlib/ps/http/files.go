package http

import (
	"fmt"
	"io/fs"
	nethttp "net/http"
	"strings"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
)

func init() {
	runner.RegisterArea(`Runtime\PS\HTTP\Files`, registerFiles)
}

// registerFiles installs net/http's three file-serving entry points.
func registerFiles(rt *runner.Runtime) {
	// HTTP\ServeFile answers $r with the file at $path, writing the content type, the byte range and the conditional headers net/http writes for one file.
	rt.RegisterFunc("HTTP\\ServeFile", serveFile(rt, "HTTP\\ServeFile"))

	// HTTP\FileServer is the handler that answers with the file under $dir the request path names, which $mux->handle takes in place of a callable.
	rt.RegisterConstructor("HTTP\\FileServer", fileServer(rt, "HTTP\\FileServer"))

	// HTTP\StripPrefix answers through $handler with $prefix cut off the request path, which a file server registered on a subtree needs.
	rt.RegisterFunc("HTTP\\StripPrefix", stripPrefix("HTTP\\StripPrefix"))
}

// RegisterGoNames installs the same three names in a Go area, as functions.
func RegisterGoNames(rt *runner.Runtime, area string) {
	// Runtime\Go\Net\HTTP\ServeFile answers $r with the file at $path, with the content type, the byte range and the conditional headers net/http writes.
	rt.RegisterFunc(area+`\ServeFile`, serveFile(rt, area+`\ServeFile`))
	// Runtime\Go\Net\HTTP\FileServer returns the handler that answers with the file under $dir the request path names.
	rt.RegisterFunc(area+`\FileServer`, fileServer(rt, area+`\FileServer`))
	// Runtime\Go\Net\HTTP\StripPrefix returns a handler answering through $handler with $prefix cut off the request path.
	rt.RegisterFunc(area+`\StripPrefix`, stripPrefix(area+`\StripPrefix`))
}

// serveFile writes one file of the source filesystem to the response.
func serveFile(rt *runner.Runtime, name string) func(w, r any, path string) error {
	return func(w, r any, path string) error {
		writer, request, err := exchange(name, w, r)
		if err != nil {
			return err
		}
		tree, file, err := rooted(rt, name, path)
		if err != nil {
			return err
		}
		nethttp.ServeFileFS(writer, request, tree, file)
		return nil
	}
}

// fileServer answers with the handler that serves the subtree at $dir.
func fileServer(rt *runner.Runtime, name string) func(dir string) (nethttp.Handler, error) {
	return func(dir string) (nethttp.Handler, error) {
		tree, under, err := rooted(rt, name, dir)
		if err != nil {
			return nil, err
		}
		sub, err := fs.Sub(tree, under)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return nethttp.FileServerFS(sub), nil
	}
}

// stripPrefix answers with the handler that cuts the mount off the path.
func stripPrefix(name string) func(prefix string, handler any) (nethttp.Handler, error) {
	return func(prefix string, handler any) (nethttp.Handler, error) {
		given, ok := handler.(nethttp.Handler)
		if !ok {
			return nil, fmt.Errorf("%s: $handler is %s, and takes a handler such as HTTP\\FileServer", name, phpType(handler))
		}
		return nethttp.StripPrefix(prefix, given), nil
	}
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

// phpType names a value for a refusal, in php's word or the go type.
func phpType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case int, int64:
		return "int"
	case float64:
		return "float"
	case string, []byte:
		return "string"
	}
	if modelutil.IsCollection(v) {
		return "array"
	}
	// A handler, a writer and a request reach a script as the go value the
	// binding answered with, and naming that type says which binding to call.
	return fmt.Sprintf("%T", v)
}
