package route

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

var testRouteFileSystem = fstest.MapFS{
	"users/show.php": {Data: []byte(`<?php
// @route GET /users/{id}
echo $_PATH["id"];
`)},
	"submit.php": {Data: []byte(`<?php
// @route: /submit
echo $_POST["name"];
`)},
	"ignored.txt": {Data: []byte(`// @route GET /ignored`)},
}

func TestServiceRegistersAnnotatedPHPFiles(t *testing.T) {
	mux := http.NewServeMux()
	svc, err := NewService(testRouteFileSystem, mux)
	if err != nil {
		t.Fatal(err)
	}
	if svc.mux != mux {
		t.Fatal("NewService did not use provided mux")
	}

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != "42" {
		t.Fatalf("body = %q, want %q", got, "42")
	}
}

func TestServiceRegistersDefaultGetAndPost(t *testing.T) {
	mux := http.NewServeMux()
	if _, err := NewService(testRouteFileSystem, mux); err != nil {
		t.Fatal(err)
	}

	body := "name=bob"
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != "bob" {
		t.Fatalf("body = %q, want %q", got, "bob")
	}
}
