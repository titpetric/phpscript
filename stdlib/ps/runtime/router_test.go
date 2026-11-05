package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	stdruntime "github.com/titpetric/phpscript/stdlib/ps/runtime"
)

// mux is a Registrar over a standard library mux, the router a host mounts.
type mux struct{ *http.ServeMux }

func (m mux) Handle(method, path string, handler http.Handler) {
	m.ServeMux.Handle(method+" "+path, handler)
}

// run executes testdata/<name> on a runtime that Install gave the Bind binding,
// and returns what it echoed and the error it ended with.
func run(t *testing.T, name string, router *stdruntime.Router) (string, error) {
	t.Helper()
	src, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	if router != nil {
		router.Install(rt)
	}
	program, err := rt.Load(string(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = rt.Run(program)
	return out.String(), err
}

func attached() (*stdruntime.Router, *http.ServeMux) {
	router, serve := stdruntime.NewRouter(), http.NewServeMux()
	router.Attach(mux{serve})
	return router, serve
}

// TestRouter_Bind pins that the routes a Bind callback registers, plain and
// grouped, reach the attached router, and that a group's middleware stays in it.
func TestRouter_Bind(t *testing.T) {
	t.Parallel()
	router, serve := attached()
	if _, err := run(t, "bind.php", router); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ path, want, group string }{
		{"/ping", "pong", ""},
		{"/api/users/7", "user 7", "api"},
	} {
		response := httptest.NewRecorder()
		serve.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || response.Body.String() != test.want {
			t.Errorf("%s = %d %q, want 200 %q", test.path, response.Code, response.Body.String(), test.want)
		}
		if got := response.Header().Get("X-Group"); got != test.group {
			t.Errorf("%s X-Group = %q, want %q", test.path, got, test.group)
		}
	}
}

// TestRouter_Refusals pins the two RuntimeExceptions a startup job can earn:
// no router attached, and a pattern without a method.
func TestRouter_Refusals(t *testing.T) {
	t.Parallel()
	_, err := run(t, "bind.php", stdruntime.NewRouter())
	if err == nil || !strings.Contains(err.Error(), "no router is mounted") {
		t.Errorf("unattached = %v, want no router is mounted", err)
	}
	router, _ := attached()
	_, err = run(t, "no_method.php", router)
	if err == nil || !strings.Contains(err.Error(), "needs a method") {
		t.Errorf("no method = %v, want needs a method", err)
	}
}

// TestRouter_OutOfScope pins that the mux is the router's for as long as Bind
// runs: a handle on it afterwards throws and the mux has no way to list routes.
func TestRouter_OutOfScope(t *testing.T) {
	t.Parallel()
	router, serve := attached()
	out, err := run(t, "out_of_scope.php", router)
	if err != nil {
		t.Fatal(err)
	}
	if out != "late refused\nno listing\n" {
		t.Errorf("output = %q", out)
	}
	response := httptest.NewRecorder()
	serve.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/late", nil))
	if response.Code != http.StatusNotFound {
		t.Errorf("/late = %d, want 404", response.Code)
	}
}

// TestRegister pins the binding a request runtime has: it throws.
func TestRegister(t *testing.T) {
	t.Parallel()
	out, err := run(t, "request.php", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "caught" {
		t.Errorf("output = %q, want caught", out)
	}
}

// TestRegistrar pins that a Bind route reaches the method and path the
// Registrar was handed, split from the pattern.
func TestRegistrar(t *testing.T) {
	t.Parallel()
	router := stdruntime.NewRouter()
	var got []string
	router.Attach(recorder(func(method, path string) { got = append(got, method+" "+path) }))
	if _, err := run(t, "bind.php", router); err != nil {
		t.Fatal(err)
	}
	if want := "GET /ping,GET /api/users/{id}"; strings.Join(got, ",") != want {
		t.Errorf("registered %v, want %s", got, want)
	}
}

// TestRouter_Whitespace pins the grammar a pattern splits on: ServeMux
// documents a method and then at least one space or tab, the mux a script
// writes its routes on takes either, and the mounted router cannot take less.
func TestRouter_Whitespace(t *testing.T) {
	t.Parallel()
	router := stdruntime.NewRouter()
	var got []string
	router.Attach(recorder(func(method, path string) { got = append(got, method+" "+path) }))
	if _, err := run(t, "whitespace.php", router); err != nil {
		t.Fatal(err)
	}
	if want := "GET /tabbed,POST /spaced"; strings.Join(got, ",") != want {
		t.Errorf("registered %v, want %s", got, want)
	}
}

type recorder func(method, path string)

func (r recorder) Handle(method, path string, _ http.Handler) { r(method, path) }

var _ stdruntime.Registrar = recorder(nil)
