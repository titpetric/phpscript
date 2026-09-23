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

	chi "github.com/go-chi/chi/v5"
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

func TestHandlerServesPublicFilesAndPHP(t *testing.T) {
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
	handler, err := NewHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
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
// middleware and the front end the platform would wire, and hands the observer
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
	router := chi.NewRouter()
	router.Use(tracer.Middleware)
	if err := telemetry.Mount(router, tracer); err != nil {
		return nil, nil, err
	}
	router.Handle("/*", handler)
	return router, recorder, nil
}

func TestHandlerDoesNotExposeProjectFilesOrPublicAnnotations(t *testing.T) {
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
// options it hands the platform from.
func TestPlatformOptionsComeFromTheServerBlock(t *testing.T) {
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

// operatorConfig writes the file the server is started with, and returns its
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

// TestReload covers what SIGHUP does, driving Reload directly so nothing
// depends on signal delivery timing.
func TestReload(t *testing.T) {
	a := site(t, "a", "a")
	b := site(t, "b", "b")

	header := "telemetry:\n  enabled: false\nserver:\n  addr: \"127.0.0.1:0\"\n"
	oneSite := header + "virtualhost:\n  - domain: a.localhost\n    root: " + a + "\n"
	twoSites := oneSite + "  - domain: b.localhost\n    root: " + b + "\n"

	start := func(t *testing.T, filename string) *managerFixture {
		t.Helper()

		globals := &flags.Options{ConfigFile: filename}
		appConfig, err := config.Load(filename)
		if err != nil {
			t.Fatal(err)
		}

		manager, err := newManager(t.Context(), nil, appConfig, globals)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(manager.Stop)
		if err := manager.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		return &managerFixture{manager: manager, filename: filename}
	}

	t.Run("a reload reads the file again", func(t *testing.T) {
		filename := operatorConfig(t, oneSite)
		fixture := start(t, filename)

		if status, got := body(t, fixture.manager.URL(), "a.localhost"); status != 200 || got != "a" {
			t.Fatalf("a.localhost = %d %q", status, got)
		}
		if status, _ := body(t, fixture.manager.URL(), "b.localhost"); status != 404 {
			t.Fatalf("b.localhost = %d, want 404 before the reload", status)
		}

		fixture.rewrite(t, twoSites)
		if err := fixture.manager.Reload(t.Context()); err != nil {
			t.Fatal(err)
		}

		// The site the edit added, and the one that was already there.
		if status, got := body(t, fixture.manager.URL(), "b.localhost"); status != 200 || got != "b" {
			t.Fatalf("b.localhost = %d %q, want the reloaded site", status, got)
		}
		if status, got := body(t, fixture.manager.URL(), "a.localhost"); status != 200 || got != "a" {
			t.Fatalf("a.localhost = %d %q", status, got)
		}
	})

	t.Run("the address survives a reload", func(t *testing.T) {
		filename := operatorConfig(t, oneSite)
		fixture := start(t, filename)

		before := fixture.manager.URL()
		fixture.rewrite(t, twoSites)
		if err := fixture.manager.Reload(t.Context()); err != nil {
			t.Fatal(err)
		}
		if after := fixture.manager.URL(); after != before {
			t.Fatalf("url = %q, want %q", after, before)
		}
	})

	// The reason Check exists. A reload that cannot be applied leaves the
	// generation that is serving alone, rather than stopping it first and
	// then discovering the new configuration is unusable.
	t.Run("a broken configuration is refused and keeps serving", func(t *testing.T) {
		filename := operatorConfig(t, oneSite)
		fixture := start(t, filename)

		serving := fixture.manager.Platform()

		fixture.rewrite(t, twoSites+"  - domain: c.localhost\n    root: /nonexistent-root\n")
		err := fixture.manager.Reload(t.Context())
		if err == nil {
			t.Fatal("a reload with a missing root was applied")
		}

		if fixture.manager.Platform() != serving {
			t.Fatal("the generation that was serving was replaced by a refused reload")
		}
		if status, got := body(t, fixture.manager.URL(), "a.localhost"); status != 200 || got != "a" {
			t.Fatalf("a.localhost = %d %q, want the old generation still serving", status, got)
		}
	})
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
