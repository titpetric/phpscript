package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

var testFS = fstest.MapFS{
	"public/index.php":  {Data: []byte(`<?php echo "home";`)},
	"public/direct.php": {Data: []byte(`<?php echo $_GET["name"];`)},
	"public/spans.php":  {Data: []byte(`<?php $span = start_span("getUser", "database"); $span->set_attribute("user_id", 42); $span->set_source("custom.php", 12); $span->record_error(new Exception("failed", 500)); $span->end(); echo "spans";`)},
	"public/early.php":  {Data: []byte(`<?php echo "early"; exit;`)},
	"public/failed.php": {Data: []byte(`<?php echo "failed"; exit(1);`)},
	"public/style.css":  {Data: []byte(`body { color: red; }`)},
	"public/app.js":     {Data: []byte(`console.log("ok");`)},
	"public/annotated.php": {Data: []byte(`<?php
// @route GET /hidden-annotation
echo "public annotation";
`)},
	"routes/hello.php": {Data: []byte(`<?php
// @route GET /hello/{name}
echo "hello " . $_REQUEST["name"];
`)},
	"secret.txt": {Data: []byte(`not public`)},
}

// TestNewCommand covers what the constructor closes over. The configuration
// arrives as a pointer so one signature covers all nine commands, and the
// server reads it, so a nil pointer resolves to the zero configuration before
// the Run closure captures it. The command declares no flags of its own: the
// listen address and the document root come from the configuration file, and
// the rest are the shared flags.
func TestNewCommand(t *testing.T) {
	t.Parallel()
	command := NewCommand(nil, &flags.Options{})

	if command.Name != "server" {
		t.Errorf("Name = %q, want %q", command.Name, "server")
	}
	if command.Title != Name {
		t.Errorf("Title = %q, want %q", command.Title, Name)
	}
	if command.Bind != nil {
		t.Error("Bind is set, want nil: the server reads the configuration, not flags of its own")
	}
	if command.Run == nil {
		t.Fatal("Run is nil")
	}
}

// TestModule covers the module serving one application root. The root is a
// directory on disk and the document root is the subdirectory below it that
// HTTP reaches, so the handler serves a file inside the document root and
// never one beside it.
func TestModule(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, DefaultDocumentRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DefaultDocumentRoot, "index.php"), []byte(`<?php echo "served";`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("not public"), 0o644); err != nil {
		t.Fatal(err)
	}

	module, err := NewModule(root, DefaultDocumentRoot, runner.Options{}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if module.Name() != "phpserver" {
		t.Errorf("Name() = %q, want %q", module.Name(), "phpserver")
	}

	for _, test := range []struct {
		path string
		code int
		body string
	}{
		{path: "/index.php", code: http.StatusOK, body: "served"},
		{path: "/secret.txt", code: http.StatusNotFound},
	} {
		rr := httptest.NewRecorder()
		module.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, test.path, nil))
		if rr.Code != test.code {
			t.Errorf("GET %s status = %d, want %d", test.path, rr.Code, test.code)
		}
		if test.body != "" && rr.Body.String() != test.body {
			t.Errorf("GET %s body = %q, want %q", test.path, rr.Body.String(), test.body)
		}
	}
}

// TestModule_Mount covers the pattern the module claims. "/*" is the last
// route a router resolves, so a module mounted this way answers every path the
// routes registered ahead of it left alone, and the annotated routes of the
// same application keep their own paths.
func TestModule_Mount(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, DefaultDocumentRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DefaultDocumentRoot, "index.php"), []byte(`<?php echo "files";`), 0o644); err != nil {
		t.Fatal(err)
	}

	module, err := NewModule(root, DefaultDocumentRoot, runner.Options{}, false, false)
	if err != nil {
		t.Fatal(err)
	}

	router := newRouter()
	router.Get("/claimed", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("router"))
	})
	var mounted platform.Router = router
	if err := module.Mount(context.Background(), mounted); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	for _, test := range []struct{ path, body string }{
		{path: "/claimed", body: "router"},
		{path: "/index.php", body: "files"},
	} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, test.path, nil))
		if rr.Code != http.StatusOK || rr.Body.String() != test.body {
			t.Errorf("GET %s = %d %q, want 200 %q", test.path, rr.Code, rr.Body.String(), test.body)
		}
	}
}

func TestHandlerServesPublicFilesAndPHP(t *testing.T) {
	t.Parallel()
	h, err := NewHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path        string
		body        string
		contentType string
	}{
		{path: "/", body: "home", contentType: "text/html"},
		{path: "/direct.php?name=Ada", body: "Ada", contentType: "text/html"},
		{path: "/style.css", body: "body { color: red; }", contentType: "text/css"},
		{path: "/app.js", body: `console.log("ok");`, contentType: "text/javascript"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rr.Code != http.StatusOK || rr.Body.String() != tt.body {
				t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
			}
			if tt.contentType != "" && !strings.HasPrefix(rr.Header().Get("Content-Type"), tt.contentType) {
				t.Fatalf("Content-Type = %q, want prefix %q", rr.Header().Get("Content-Type"), tt.contentType)
			}
		})
	}
}

func TestRouteModuleServesAnnotatedRoutes(t *testing.T) {
	t.Parallel()
	handler, err := NewHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	router := newRouter()
	routes := annotations.NewRoute(testFS, annotations.WithExcludedDirectory("public"))
	if err := routes.Mount(context.Background(), router); err != nil {
		t.Fatal(err)
	}
	router.Handle("/*", handler)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/hello/Ada", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "hello Ada" {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
}

func TestHandlerDoesNotOwnDebugFrontEnd(t *testing.T) {
	t.Parallel()
	h, err := NewHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, telemetry.DefaultPath, nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
}

func TestHandlerRecordsPHPSpan(t *testing.T) {
	t.Parallel()
	h, err := newTracedHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRecorder()
	h.ServeHTTP(request, httptest.NewRequest(http.MethodGet, "/spans.php", nil))
	if request.Code != http.StatusOK || request.Body.String() != "spans" {
		t.Fatalf("status = %d, body = %q", request.Code, request.Body.String())
	}
	id := request.Header().Get(telemetry.RequestIDHeader)
	if id == "" {
		t.Fatal("Request-Id header is empty")
	}

	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, telemetry.DefaultPath+"/traces", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), telemetry.DefaultPath+"/trace/"+id) {
		t.Fatalf("status = %d, body = %q", list.Code, list.Body.String())
	}

	detail := httptest.NewRecorder()
	h.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, telemetry.DefaultPath+"/trace/"+id, nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "getUser") || !strings.Contains(detail.Body.String(), "database") || !strings.Contains(detail.Body.String(), "custom.php:L12") {
		t.Fatalf("status = %d, body = %q", detail.Code, detail.Body.String())
	}

	detailJSON := httptest.NewRecorder()
	detailRequest := httptest.NewRequest(http.MethodGet, telemetry.DefaultPath+"/trace/"+id, nil)
	detailRequest.Header.Set("Accept", "application/json")
	h.ServeHTTP(detailJSON, detailRequest)
	for _, value := range []string{`"name": "getUser"`, `"kind": "database"`, `"filename": "custom.php"`, `"line": 12`, `"user_id": 42`, `"error": "failed"`} {
		if !strings.Contains(detailJSON.Body.String(), value) {
			t.Fatalf("JSON detail does not contain %s: %s", value, detailJSON.Body.String())
		}
	}
}

// TestHandlerRecordsExitAsFailureOnlyOnANonZeroCode pins what the recorded
// state means: a page ending with exit() ran to completion, and only a
// non-zero code is a failure, because the state feeds the reported SLA.
func TestHandlerRecordsExitAsFailureOnlyOnANonZeroCode(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		path      string
		body      string
		wantError bool
	}{
		{path: "/early.php", body: "early"},
		{path: "/failed.php", body: "failed", wantError: true},
	} {
		t.Run(test.path, func(t *testing.T) {
			h, recorder, err := newTracedServer(testFS)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusOK || response.Body.String() != test.body {
				t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
			}

			traces := recorder.Tracer().Traces()
			if len(traces) != 1 {
				t.Fatalf("traces = %+v", traces)
			}
			if gotError := traces[0].State == telemetry.StateError; gotError != test.wantError {
				t.Fatalf("state = %q, wantError = %t", traces[0].State, test.wantError)
			}
		})
	}
}

func newTracedHandler(root fstest.MapFS) (http.Handler, error) {
	handler, _, err := newTracedServer(root)
	return handler, err
}

// newTracedServer stands in for the platform: it builds the recorder, wires the
// middleware and the front end the platform would wire, and passes the observer
// the tracer that recorder owns.
func newTracedServer(root fstest.MapFS) (http.Handler, *telemetry.Module, error) {
	options := telemetry.NewOptions("phpscript")
	options.Enabled = true
	options.ServiceName = "phpscript"
	tracer, err := telemetry.New(options)
	if err != nil {
		return nil, nil, err
	}
	recorder := telemetry.NewModule(tracer)
	handler, err := newHandler(root, "", DefaultDocumentRoot, runner.Options{}, false, false, recorder)
	if err != nil {
		return nil, nil, err
	}
	router := newRouter()
	router.Use(tracer.Middleware)
	if err := telemetry.Mount(router, tracer); err != nil {
		return nil, nil, err
	}
	router.Handle("/*", handler)
	return router, recorder, nil
}

func TestHandlerDoesNotExposeProjectFilesOrPublicAnnotations(t *testing.T) {
	t.Parallel()
	h, err := NewHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/secret.txt", "/routes/hello.php", "/hello/Ada", "/hidden-annotation"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, body = %q", path, rr.Code, rr.Body.String())
		}
	}
}

// TestPlatformOptionsComeFromTheServerBlock pins where the server takes the
// options it passes the platform from.
func TestPlatformOptionsComeFromTheServerBlock(t *testing.T) {
	t.Parallel()
	options, err := config.NewTestConfig().PlatformOptions()
	if err != nil {
		t.Fatal(err)
	}
	if options.ServerAddr != "127.0.0.1:0" || !options.Quiet {
		t.Fatalf("options = %+v, want what the server block sets", options)
	}
}

// site writes an application root serving body at its index, and returns the
// root.
func site(t *testing.T, name, body string) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), name)
	public := filepath.Join(root, "public")
	if err := os.MkdirAll(public, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(public, "index.php"), []byte("<?php echo \""+body+"\";"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "phpscript.yml"), []byte("telemetry:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// operatorConfig writes the file the server starts from, and returns its
// path. Rewriting it and reloading is what these tests are about.
func operatorConfig(t *testing.T, source string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// body fetches the index of one virtual host.
func body(t *testing.T, url, host string) (int, string) {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, url+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = host

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	buf := make([]byte, 64)
	n, _ := response.Body.Read(buf)
	return response.StatusCode, string(buf[:n])
}

// managerFixture is a started manager and the file it reads.
type managerFixture struct {
	manager  *platform.Manager
	filename string
}

func (f *managerFixture) rewrite(t *testing.T, source string) {
	t.Helper()

	if err := os.WriteFile(f.filename, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}
