package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

var testFS = fstest.MapFS{
	"public/index.php":  {Data: []byte(`<?php echo "home";`)},
	"public/direct.php": {Data: []byte(`<?php echo $_GET["name"];`)},
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
