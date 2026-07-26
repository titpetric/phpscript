package runner_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
)

// runReq parses src, registers a Context built from r, executes, and returns the
// output plus the staged response headers.
func runReq(t *testing.T, r *http.Request, src string) (string, http.Header) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	ctx := runner.FromRequest(r)
	ctx.Register(rt)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String(), ctx.ResponseHeaders()
}

func TestFromRequestGetAndPath(t *testing.T) {
	mux := http.NewServeMux()
	var got *http.Request
	mux.HandleFunc("GET /users/{id}", func(_ http.ResponseWriter, r *http.Request) { got = r })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/users/42?q=hello")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	out, _ := runReq(t, got, `<?php echo $_PATH["id"] . "-" . $_GET["q"];`)
	if out != "42-hello" {
		t.Fatalf("got %q", out)
	}
}

func TestFromRequestPost(t *testing.T) {
	r := httptest.NewRequest("POST", "/submit", strings.NewReader("name=bob"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	out, _ := runReq(t, r, `<?php echo $_POST["name"];`)
	if out != "bob" {
		t.Fatalf("got %q", out)
	}
}

func TestSuperglobalsVisibleInsideFunctions(t *testing.T) {
	mux := http.NewServeMux()
	var got *http.Request
	mux.HandleFunc("GET /run/{jobName}", func(_ http.ResponseWriter, r *http.Request) { got = r })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/run/api_stats")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	src := `<?php
function route_job_name() {
	return $_PATH["jobName"];
}
echo route_job_name();
`
	out, _ := runReq(t, got, src)
	if out != "api_stats" {
		t.Fatalf("got %q, want %q", out, "api_stats")
	}
}

func TestGlobalsNotVisibleInsideFunctions(t *testing.T) {
	src := `<?php
$job = "local";
function read_job() {
	return $job;
}
echo read_job();
`
	out, _ := runReq(t, httptest.NewRequest("GET", "/", nil), src)
	if out != "" {
		t.Fatalf("got %q, want empty string", out)
	}
}

func TestGetAllHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Custom", "yes")

	out, _ := runReq(t, r, `<?php $h = getallheaders(); echo $h["X-Custom"];`)
	if out != "yes" {
		t.Fatalf("got %q", out)
	}
}

func TestHeaderEmission(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)

	_, resp := runReq(t, r, `<?php header("Content-Type: application/json");`)
	if got := resp.Get("Content-Type"); got != "application/json" {
		t.Fatalf("got %q", got)
	}
}
