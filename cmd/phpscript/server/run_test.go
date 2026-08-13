package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/status"
)

var testFS = fstest.MapFS{
	"public/index.php":  {Data: []byte(`<?php echo "home";`)},
	"public/direct.php": {Data: []byte(`<?php echo $_GET["name"];`)},
	"public/spans.php":  {Data: []byte(`<?php span("getUser", "database"); echo "spans";`)},
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
		{path: "/hello/Ada", body: "hello Ada", contentType: ""},
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

func TestHandlerServesServerStatus(t *testing.T) {
	h, err := newStatusHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/debug/server-status", nil)
	req.Header.Set("Accept", "text/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"active_requests":1`) {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Request-Id") == "" {
		t.Fatal("Request-Id header is empty")
	}
}

func TestHandlerDoesNotOwnServerStatus(t *testing.T) {
	h, err := NewHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, status.ServerStatusPath, nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
}

func TestHandlerRecordsPHPSpan(t *testing.T) {
	h, err := newStatusHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRecorder()
	h.ServeHTTP(request, httptest.NewRequest(http.MethodGet, "/spans.php", nil))
	if request.Code != http.StatusOK || request.Body.String() != "spans" {
		t.Fatalf("status = %d, body = %q", request.Code, request.Body.String())
	}
	id := request.Header().Get("Request-Id")
	overview := httptest.NewRecorder()
	h.ServeHTTP(overview, httptest.NewRequest(http.MethodGet, "/debug/server-status/log", nil))
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), "/debug/server-status/detail/"+id) || !strings.Contains(overview.Body.String(), "GET /spans.php") {
		t.Fatalf("status = %d, body = %q", overview.Code, overview.Body.String())
	}

	detail := httptest.NewRecorder()
	h.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/debug/server-status/detail/"+id, nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "getUser") || !strings.Contains(detail.Body.String(), "database") {
		t.Fatalf("status = %d, body = %q", detail.Code, detail.Body.String())
	}
}

func newStatusHandler(root fstest.MapFS) (http.Handler, error) {
	serverStatus := status.NewModule(status.NewOptions())
	var options runner.Options
	handler, err := newHandler(root, "", options, false, serverStatus)
	if err != nil {
		return nil, err
	}
	router := chi.NewRouter()
	router.Use(serverStatus.Middleware)
	if err := serverStatus.Mount(context.Background(), router); err != nil {
		return nil, err
	}
	router.Handle("/*", handler)
	return router, nil
}

func TestHandlerDoesNotExposeProjectFilesOrPublicAnnotations(t *testing.T) {
	h, err := NewHandler(testFS)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/secret.txt", "/routes/hello.php", "/hidden-annotation"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, body = %q", path, rr.Code, rr.Body.String())
		}
	}
}
