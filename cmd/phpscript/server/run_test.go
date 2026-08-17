package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	chi "github.com/go-chi/chi/v5"

	routesvc "github.com/titpetric/phpscript/route"
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
echo "hello " . $_PATH["name"];
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
	routes := routesvc.NewModule(testFS, routesvc.WithExcludedDirectory("public"))
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

func TestHandlerServesDebugFrontEnd(t *testing.T) {
	h, err := newTracedHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, telemetry.DefaultPath, nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "phpscript") {
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
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), telemetry.DefaultPath+"/trace/"+id) || !strings.Contains(list.Body.String(), "GET /spans.php") {
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

func newTracedServer(root fstest.MapFS) (http.Handler, *telemetry.Module, error) {
	options := telemetry.NewOptions()
	options.ServiceName = "phpscript"
	recorder, err := telemetry.NewModule(options)
	if err != nil {
		return nil, nil, err
	}
	handler, err := newHandler(root, "", runner.Options{}, false, recorder)
	if err != nil {
		return nil, nil, err
	}
	router := chi.NewRouter()
	router.Use(recorder.Middleware)
	if err := recorder.Mount(context.Background(), router); err != nil {
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
