package http_test

import (
	"context"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// serveScriptIn runs src with the source filesystem rooted at dir and answers
// the mux the script left in its MUX constant.
func serveScriptIn(t *testing.T, dir, src string) (*strings.Builder, nethttp.Handler) {
	t.Helper()
	rt, out := runtimeIn(t, dir)
	program, err := rt.Load(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v (output %q)", err, out.String())
	}
	value, ok := rt.Const("MUX")
	if !ok {
		t.Fatal("the script defined no MUX constant")
	}
	handler, ok := value.(nethttp.Handler)
	if !ok {
		t.Fatalf("MUX is %T, want an nethttp.Handler", value)
	}
	return out, handler
}

// runScriptIn runs src in dir and answers the output, a caught message included.
func runScriptIn(t *testing.T, dir, src string) string {
	t.Helper()
	rt, out := runtimeIn(t, dir)
	program, err := rt.Load(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		return err.Error()
	}
	return out.String()
}

func runtimeIn(t *testing.T, dir string) (*runner.Runtime, *strings.Builder) {
	t.Helper()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{RootFS: os.DirFS(dir)})
	stdlib.Register(rt)
	rt.SetContext(context.Background())
	return rt, &out
}

// tree writes a source tree a script serves out of and returns its root.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range map[string]string{
		"public/hello.txt":     "hello\n",
		"public/style.css":     "body{color:red}\n",
		"public/deep/page.txt": "deep\n",
		"secret.php":           "<?php $key = 1;\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestServeFile covers the binding a PHP handler answers one file through: the
// bytes, the content type net/http sniffs, and a conditional request.
func TestServeFile(t *testing.T) {
	t.Parallel()
	_, handler := serveScriptIn(t, tree(t), `<?php
$mux = new HTTP\Mux();

$mux->handle("GET /files/{name...}", function ($w, $r) {
	HTTP\ServeFile($w, $r, "public/" . $r->path_value("name"));
});

define("MUX", $mux);`)

	for _, tc := range []struct {
		name   string
		path   string
		status int
		want   string
		kind   string
	}{
		{"a file", "/files/hello.txt", nethttp.StatusOK, "hello\n", "text/plain; charset=utf-8"},
		{"the type off the extension", "/files/style.css", nethttp.StatusOK, "body{color:red}\n", "text/css; charset=utf-8"},
		{"a file below", "/files/deep/page.txt", nethttp.StatusOK, "deep\n", "text/plain; charset=utf-8"},
		{"a name that is not there", "/files/nope.txt", nethttp.StatusNotFound, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(nethttp.MethodGet, tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if tc.want != "" && w.Body.String() != tc.want {
				t.Errorf("body %q, want %q", w.Body.String(), tc.want)
			}
			if tc.kind != "" && w.Header().Get("Content-Type") != tc.kind {
				t.Errorf("type %q, want %q", w.Header().Get("Content-Type"), tc.kind)
			}
		})
	}
}

// TestServeFileStaysInTheTree: a path climbing out of the source filesystem
// names nothing, the one rule a binding publishing a tree has to hold.
func TestServeFileStaysInTheTree(t *testing.T) {
	t.Parallel()
	_, handler := serveScriptIn(t, tree(t), `<?php
$mux = new HTTP\Mux();

$mux->handle("GET /out/{name...}", function ($w, $r) {
	HTTP\ServeFile($w, $r, $r->path_value("name"));
});

define("MUX", $mux);`)

	for _, path := range []string{
		"/out/../../../etc/passwd",
		"/out/public/../../etc/passwd",
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(nethttp.MethodGet, path, nil))
		if w.Code == nethttp.StatusOK && strings.Contains(w.Body.String(), "root:") {
			t.Errorf("%s answered the host's file: %q", path, w.Body.String())
		}
	}
}

// TestFileServer covers the handler registered in place of a callable: no PHP
// runs per request, and the subtree bounds every name it answers.
func TestFileServer(t *testing.T) {
	t.Parallel()
	_, handler := serveScriptIn(t, tree(t), `<?php
$mux = new HTTP\Mux();

$mux->handle("GET /assets/", HTTP\StripPrefix("/assets/", new HTTP\FileServer("public")));

define("MUX", $mux);`)

	for _, tc := range []struct {
		name   string
		path   string
		status int
		want   string
	}{
		{"a file of the subtree", "/assets/hello.txt", nethttp.StatusOK, "hello\n"},
		{"a file below it", "/assets/deep/page.txt", nethttp.StatusOK, "deep\n"},
		// The mux cleans the path and the escape never reaches the tree.
		{"a file above it", "/assets/../secret.php", nethttp.StatusTemporaryRedirect, ""},
		{"a name that is not there", "/assets/nope.txt", nethttp.StatusNotFound, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(nethttp.MethodGet, tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if tc.want != "" && w.Body.String() != tc.want {
				t.Errorf("body %q, want %q", w.Body.String(), tc.want)
			}
		})
	}
}

// TestFileServerRefusals: each binding names the value it got, where a script
// passes something that is no handler or a path with no tree behind it.
func TestFileServerRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"a prefix over a closure", `HTTP\StripPrefix("/a/", function ($w, $r) {});`, "takes a handler such as HTTP\\FileServer"},
		{"a handler under a middleware", "$mux = new HTTP\\Mux();\n$mux->use(function ($next) { return $next; });\n$mux->handle(\"GET /a/\", new HTTP\\FileServer(\"public\"));", "takes no PHP middleware"},
		{"a server with no path", `new HTTP\FileServer("");`, "a path is required"},
		{"a file served without a writer", `HTTP\ServeFile("w", "r", "public/hello.txt");`, "$w is string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runScriptIn(t, tree(t), "<?php\ntry {\n"+tc.src+"\n} catch (Throwable $e) {\n  echo $e->getMessage();\n}")
			if !strings.Contains(err, tc.want) {
				t.Errorf("want %q in %q", tc.want, err)
			}
		})
	}
}
