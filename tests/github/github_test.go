// Package github_test holds one regression test per closed GitHub issue that
// the .phpt fixture runner cannot reach.
//
// A fixture runs a script. An issue about route registration, a server flag or
// a host binding needs the machinery around the script, so it is covered here
// instead, named Test_IssueNNN after the issue it closes.
// tests/fixtures/github/NNN.yml keeps the transcript that was reported.
package github_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	chi "github.com/go-chi/chi/v5"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/lint"
)

// routers registers routes on both routers the runtime supports and returns
// them by name. Each entry of routes is a file name and its @route comment;
// every endpoint echoes $_REQUEST, which is what these tests read; the
// requests carry no query, form or cookie fields, so what comes back is the
// path values alone.
func routers(t *testing.T, routes map[string]string) map[string]http.Handler {
	t.Helper()
	sources := fstest.MapFS{}
	for name, annotation := range routes {
		sources[name] = &fstest.MapFile{
			Data: []byte("<?php\n" + annotation + "\necho json_encode($_REQUEST);\n"),
		}
	}

	mux := http.NewServeMux()
	if err := annotations.NewRoute(sources).RegisterMux(mux); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	if err := annotations.NewRoute(sources).Mount(context.Background(), router); err != nil {
		t.Fatal(err)
	}
	return map[string]http.Handler{"servemux": mux, "chi": router}
}

func get(t *testing.T, handler http.Handler, url string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, url, nil))
	body, err := io.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	return rr.Code, string(body)
}

// lintRoutes returns the route diagnostics phpscript lint reports for src.
func lintRoutes(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	diags, err := lint.File("route.php", src)
	if err != nil {
		t.Fatal(err)
	}
	var out []lint.Diagnostic
	for _, d := range diags {
		if strings.Contains(d.Message, "route") {
			out = append(out, d)
		}
	}
	return out
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
