package runner_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestRequestSuperglobalMerge pins what $_REQUEST holds and the order the
// merge takes: query, form and cookie fields as PHP merges them, and the route's
// path values written over all three, which is the documented deviation.
func TestRequestSuperglobalMerge(t *testing.T) {
	t.Parallel()
	// The handler builds the context, while the body is still readable; after
	// the handler returns the server has closed it.
	mux := http.NewServeMux()
	var ctx runner.Context
	mux.HandleFunc("POST /users/{id}", func(_ http.ResponseWriter, r *http.Request) {
		ctx = runner.FromRequest(r)
	})

	r := httptest.NewRequest("POST", "/users/42?id=from-get&a=get&b=get&c=get", strings.NewReader("id=from-post&b=post"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Cookie", "c=cookie")
	mux.ServeHTTP(httptest.NewRecorder(), r)

	src := `<?php echo $_REQUEST["id"], "|", $_REQUEST["a"], "|", $_REQUEST["b"], "|", $_REQUEST["c"];`
	out := runCtx(t, ctx, src)
	if want := "42|get|post|cookie"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestRequestSuperglobalIsACopy pins that $_REQUEST is its own array, as in
// PHP: writing through it must not reach $_GET, and the other way around.
func TestRequestSuperglobalIsACopy(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/?a=1", nil)
	src := `<?php
$_REQUEST["a"] = "changed";
$_GET["b"] = "added";
echo $_GET["a"], "|", isset($_REQUEST["b"]) ? "leaked" : "own";
`
	out, _ := runReq(t, r, src)
	if want := "1|own"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestSuperglobalsVisibleInsideFunctions(t *testing.T) {
	t.Parallel()
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
	return $_REQUEST["jobName"];
}
echo route_job_name();
`
	out, _ := runReq(t, got, src)
	if out != "api_stats" {
		t.Fatalf("got %q, want %q", out, "api_stats")
	}
}

// TestServerSuperglobalStrings pins that every other key reaches a script as a
// string, as $_SERVER holds.
func TestServerSuperglobalStrings(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("POST", "/submit?a=1", strings.NewReader("name=bob"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = "127.0.0.1:56138"

	out := runCtx(t, runner.FromRequest(r), `<?php
$keys = ["REQUEST_METHOD", "REQUEST_URI", "QUERY_STRING", "SERVER_PROTOCOL", "HTTP_HOST",
	"REMOTE_ADDR", "REMOTE_PORT", "REQUEST_SCHEME", "CONTENT_TYPE", "CONTENT_LENGTH"];
foreach ($keys as $key) {
	if (!is_string($_SERVER[$key])) {
		echo $key, " is not a string\n";
	}
}
echo $_SERVER["REMOTE_PORT"], "|", $_SERVER["CONTENT_LENGTH"], "|", $_SERVER["REQUEST_SCHEME"];`)
	if want := "56138|8|http"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// A hand-assembled Context has no request to decode, so Register promotes its
// flat maps as they stand: the fixture harness, a CLI run, an embedder.
func TestRegisterFallsBackToFlatMaps(t *testing.T) {
	t.Parallel()
	c := runner.NewContext()
	c.Get["greeting"] = "hello"
	c.Post["message"] = "form"
	c.Cookie["session"] = "abc"

	out := runCtx(t, c, `<?php echo $_GET["greeting"], $_POST["message"], $_COOKIE["session"];`)
	if want := "helloformabc"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
