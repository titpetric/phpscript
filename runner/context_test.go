package runner_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/mapmap"
	"github.com/titpetric/phpscript/stdlib"
)

// runReq parses src, registers a Context built from r, executes, and returns the
// output plus the staged response headers.
func runReq(t *testing.T, r *http.Request, src string) (string, http.Header) {
	t.Helper()
	ctx := runner.FromRequest(r)
	return runCtx(t, ctx, src), ctx.ResponseHeaders()
}

// runCtx executes src against an already built request context, for the cases
// that inspect the same context from both PHP and Go.
func runCtx(t *testing.T, ctx runner.Context, src string) string {
	t.Helper()
	return runSAPI(t, ctx, "", src)
}

// runSAPI is runCtx under a named SAPI, for the cases that answer differently
// on the command line than they do for a request.
func runSAPI(t *testing.T, ctx runner.Context, sapi, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{SAPI: sapi})
	stdlib.Register(rt)
	ctx.Register(rt)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

func TestFromRequestGetAndPath(t *testing.T) {
	t.Parallel()
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

	out, _ := runReq(t, got, `<?php echo $_REQUEST["id"] . "-" . $_GET["q"];`)
	if out != "42-hello" {
		t.Fatalf("got %q", out)
	}
}

func TestFromRequestPost(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("POST", "/submit", strings.NewReader("name=bob"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	out, _ := runReq(t, r, `<?php echo $_POST["name"];`)
	if out != "bob" {
		t.Fatalf("got %q", out)
	}
}

// TestFromRequestRepeatedValues pins the flattening rule for a key sent more
// than once. PHP's parser assigns each repetition over the one before it, so
// the last value is the one a script sees.
func TestFromRequestRepeatedValues(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("POST", "/submit?tag=a&tag=b", strings.NewReader("name=bob&name=alice"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	out, _ := runReq(t, r, `<?php echo $_GET["tag"] . "|" . $_POST["name"];`)
	if want := "b|alice"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// multipartFile is one file part of a request body built by multipartRequest.
type multipartFile struct {
	field    string
	filename string
	content  string
}

// multipartRequest builds a POST carrying a multipart/form-data body, the shape
// a browser sends for a form with a file input.
func multipartRequest(t *testing.T, values map[string]string, files ...multipartFile) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range values {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	for _, f := range files {
		part, err := w.CreateFormFile(f.field, f.filename)
		if err != nil {
			t.Fatalf("create part: %v", err)
		}
		if _, err := part.Write([]byte(f.content)); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	r := httptest.NewRequest("POST", "/upload", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r
}

// TestFromRequestUrlencodedUnaffected pins that the content type alone sends a
// request down the multipart branch: an urlencoded POST resolves through ParseForm
// and reports no uploads.
func TestFromRequestUrlencodedUnaffected(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("POST", "/submit", strings.NewReader("name=bob"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx := runner.FromRequest(r)
	if got := ctx.PostMap()["name"]; got != "bob" {
		t.Fatalf("$_POST[name] = %q", got)
	}
	if len(ctx.FileMap()) != 0 {
		t.Fatalf("files = %v, want none", ctx.FileMap())
	}
}

func TestGlobalsNotVisibleInsideFunctions(t *testing.T) {
	t.Parallel()
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

// wantServer checks the $_SERVER keys a request produced. A want value of ""
// asserts the key is absent, so PHP says "not this kind of request":
// no HTTPS on a plain one, no CONTENT_LENGTH on a chunked one.
func wantServer(t *testing.T, server *mapmap.MapMap, want map[string]string) {
	t.Helper()
	for key, value := range want {
		read := server.Read(key)
		got, _ := read.(string)
		ok := server.Has(key)
		if value == "" {
			if ok {
				t.Errorf("$_SERVER[%s] = %q, want it unset", key, got)
			}
			continue
		}
		if got != value {
			t.Errorf("$_SERVER[%s] = %q, want %q", key, got, value)
		}
	}
}

// TestServerVarsPlainGet covers the keys a bodyless HTTP request produces. The
// expected shape is what php 8.5's built-in server writes for the same request,
// less the keys that need a document root and a script file.
func TestServerVarsPlainGet(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/index.php?a=1&b=two", nil)
	r.RemoteAddr = "127.0.0.1:56138"

	ctx := runner.FromRequest(r)
	wantServer(t, ctx.ServerMap(), map[string]string{
		"REQUEST_METHOD":  "GET",
		"REQUEST_URI":     "/index.php?a=1&b=two",
		"QUERY_STRING":    "a=1&b=two",
		"SERVER_PROTOCOL": "HTTP/1.1",
		"HTTP_HOST":       "example.com",
		"REMOTE_ADDR":     "127.0.0.1",
		"REMOTE_PORT":     "56138",
		"REQUEST_SCHEME":  "http",
		// A request with no body and no headers describing one has neither
		// key, and a plain request has no HTTPS at all.
		"HTTPS":          "",
		"CONTENT_TYPE":   "",
		"CONTENT_LENGTH": "",
	})
}

// TestServerVarsRemoteAddr covers the split of Go's "address:port" into the two
// keys PHP has for it. An IPv6 peer arrives bracketed and lands the way PHP
// writes it, without brackets.
func TestServerVarsRemoteAddr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		remote string
		addr   string
		port   string
	}{
		{remote: "127.0.0.1:56138", addr: "127.0.0.1", port: "56138"},
		{remote: "[::1]:37154", addr: "::1", port: "37154"},
		// A Go host may put anything in the field; one without a port lands
		// whole, and the split guesses at nothing.
		{remote: "unix-socket", addr: "unix-socket", port: ""},
		{remote: "", addr: "", port: ""},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.remote
		ctx := runner.FromRequest(r)
		wantServer(t, ctx.ServerMap(), map[string]string{
			"REMOTE_ADDR": tc.addr,
			"REMOTE_PORT": tc.port,
		})
	}
}

// TestServerVarsContentHeaders covers the two keys that describe a body. PHP
// mirrors the headers and not the body, so a GET that sent a content type
// has CONTENT_TYPE, and a body of zero bytes that announced its length has
// CONTENT_LENGTH "0".
func TestServerVarsContentHeaders(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("POST", "/submit", strings.NewReader("name=bob&x=1"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx := runner.FromRequest(r)
	wantServer(t, ctx.ServerMap(), map[string]string{
		"REQUEST_METHOD": "POST",
		"CONTENT_TYPE":   "application/x-www-form-urlencoded",
		"CONTENT_LENGTH": "12",
		// PHP mirrors every header into HTTP_*, these two included.
		"HTTP_CONTENT_TYPE": "application/x-www-form-urlencoded",
	})

	// A content type without a body still reaches CONTENT_TYPE.
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Content-Type", "text/plain")
	wantServer(t, runner.FromRequest(r).ServerMap(), map[string]string{
		"CONTENT_TYPE":   "text/plain",
		"CONTENT_LENGTH": "",
	})

	// An announced empty body has a length; PHP writes the "0".
	r = httptest.NewRequest("POST", "/", strings.NewReader(""))
	r.Header.Set("Content-Type", "text/plain")
	r.Header.Set("Content-Length", "0")
	wantServer(t, runner.FromRequest(r).ServerMap(), map[string]string{
		"CONTENT_LENGTH": "0",
	})

	// A chunked body announces no length, and PHP leaves the key out.
	r = httptest.NewRequest("POST", "/", strings.NewReader("hello"))
	r.Header.Set("Content-Type", "text/plain")
	r.ContentLength = -1
	wantServer(t, runner.FromRequest(r).ServerMap(), map[string]string{
		"CONTENT_TYPE":   "text/plain",
		"CONTENT_LENGTH": "",
	})
}

// TestServerVarsTLS covers a request this process accepted over TLS. PHP sets
// HTTPS only then, and to "on"; an X-Forwarded-Proto is the client's word and
// changes neither key.
func TestServerVarsTLS(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	var got *http.Request
	mux.HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) { got = r })
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	wantServer(t, runner.FromRequest(got).ServerMap(), map[string]string{
		"REQUEST_SCHEME": "https",
		"HTTPS":          "on",
	})

	plain := httptest.NewRequest("GET", "/", nil)
	plain.Header.Set("X-Forwarded-Proto", "https")
	wantServer(t, runner.FromRequest(plain).ServerMap(), map[string]string{
		"REQUEST_SCHEME": "http",
		"HTTPS":          "",
		// The header is still readable, as any other header is.
		"HTTP_X_FORWARDED_PROTO": "https",
	})
}

// TestServerVarsRequestTime covers the request start time. The seconds key is
// the whole second of the float one, and PHP types the first as an integer and
// the second as a float where every other key holds a string.
func TestServerVarsRequestTime(t *testing.T) {
	t.Parallel()
	before := time.Now()
	ctx := runner.FromRequest(httptest.NewRequest("GET", "/", nil))
	after := time.Now()

	seconds, err := strconv.ParseInt(ctx.Server["REQUEST_TIME"], 10, 64)
	if err != nil {
		t.Fatalf("REQUEST_TIME = %q: %v", ctx.Server["REQUEST_TIME"], err)
	}
	if seconds < before.Unix() || seconds > after.Unix() {
		t.Fatalf("REQUEST_TIME = %d, want between %d and %d", seconds, before.Unix(), after.Unix())
	}
	exact, err := strconv.ParseFloat(ctx.Server["REQUEST_TIME_FLOAT"], 64)
	if err != nil {
		t.Fatalf("REQUEST_TIME_FLOAT = %q: %v", ctx.Server["REQUEST_TIME_FLOAT"], err)
	}
	if int64(exact) != seconds {
		t.Fatalf("REQUEST_TIME_FLOAT = %f, want the same second as REQUEST_TIME %d", exact, seconds)
	}

	out := runCtx(t, ctx, `<?php
echo is_int($_SERVER["REQUEST_TIME"]) ? "int" : "not-int", "|";
$float = $_SERVER["REQUEST_TIME_FLOAT"];
echo !is_int($float) && !is_string($float) && is_numeric($float) ? "float" : "not-float", "|";
echo $_SERVER["REQUEST_TIME"] === (int)$_SERVER["REQUEST_TIME_FLOAT"] ? "same-second" : "differs";`)
	if want := "int|float|same-second"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestGetAllHeaders(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Custom", "yes")

	out, _ := runReq(t, r, `<?php $h = getallheaders(); echo $h["X-Custom"];`)
	if out != "yes" {
		t.Fatalf("got %q", out)
	}
}

func TestHeaderEmission(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)

	_, resp := runReq(t, r, `<?php header("Content-Type: application/json");`)
	if got := resp.Get("Content-Type"); got != "application/json" {
		t.Fatalf("got %q", got)
	}
}

func TestLocationHeaderSetsRedirectStatus(t *testing.T) {
	t.Parallel()
	ctx := runner.NewContext()
	rt := runner.New(&strings.Builder{}, runner.Options{})
	// header() is a standard-library binding that finds this Context through
	// the runtime context, so this test registers the library the way a host
	// does: stdlib first, request second.
	stdlib.Register(rt)
	ctx.Register(rt)
	prog, err := parser.Parse(`<?php header("Location: /next");`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(prog); err != nil {
		t.Fatal(err)
	}
	if got := ctx.ResponseStatus(); got != http.StatusFound {
		t.Fatalf("status = %d, want %d", got, http.StatusFound)
	}
}

// TestStatusLineHeaderSetsStatus pins the other spelling header() takes.
// "HTTP/1.0 404 Not Found" carries a status alone, with no name and no value,
// so without this branch it falls out for want of a name:value shape.
func TestStatusLineHeaderSetsStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		header string
		want   int
	}{
		{header: `HTTP/1.0 404 Not Found`, want: http.StatusNotFound},
		{header: `HTTP/1.1 503 Service Unavailable`, want: http.StatusServiceUnavailable},
		{header: `HTTP/1.1 204`, want: http.StatusNoContent},

		// Not status lines. The first is a header named Status, and the rest
		// name no status this response could carry.
		{header: `Status: 404`, want: 0},
		{header: `not a header at all`, want: 0},
		{header: `HTTP/1.1 twenty`, want: 0},
		{header: `HTTP/1.1 99`, want: 0},
		{header: `HTTP/1.1`, want: 0},
	}

	for _, test := range tests {
		t.Run(test.header, func(t *testing.T) {
			ctx := runner.NewContext()
			runCtx(t, ctx, `<?php header(`+strconv.Quote(test.header)+`);`)
			if got := ctx.ResponseStatus(); got != test.want {
				t.Fatalf("status = %d, want %d", got, test.want)
			}
		})
	}
}

// TestHTTPResponseCode pins PHP's two answers. A web request starts out
// answering 200, so there is always a status to report and always a previous
// one to return. On the command line a script starts with none, which is
// what the false and the true are: nothing to report, and no previous status to
// return for the first one set.
//
// json_encode keeps the types visible; phpscript has no var_dump.
func TestHTTPResponseCode(t *testing.T) {
	t.Parallel()
	const src = `<?php
echo json_encode([
	http_response_code(),
	http_response_code(404),
	http_response_code(),
	http_response_code(503),
]);`

	tests := []struct {
		sapi string
		want string
	}{
		{sapi: "cgi-phpscript", want: "[200,200,404,404]"},
		{sapi: "http", want: "[200,200,404,404]"},
		{sapi: "cli", want: "[false,true,404,404]"},
		// A host that named no SAPI is not serving a request, so it starts out
		// the way the command line does.
		{sapi: "", want: "[false,true,404,404]"},
	}

	for _, test := range tests {
		t.Run(test.sapi, func(t *testing.T) {
			ctx := runner.NewContext()
			if got := runSAPI(t, ctx, test.sapi, src); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
			if got := ctx.ResponseStatus(); got != http.StatusServiceUnavailable {
				t.Fatalf("staged status = %d, want 503", got)
			}
		})
	}
}

// TestHTTPResponseCodeIgnoresANonStatus pins that a zero, and anything that is
// not a number at all, is not a status: PHP reads the call as the reporting one
// and leaves the response alone.
func TestHTTPResponseCodeIgnoresANonStatus(t *testing.T) {
	t.Parallel()
	ctx := runner.NewContext()
	got := runSAPI(t, ctx, "cgi-phpscript", `<?php
http_response_code(404);
echo json_encode([
	http_response_code(0),
	http_response_code(null),
	http_response_code(false),
	http_response_code("nonsense"),
	http_response_code(),
]);`)
	if want := "[404,404,404,404,404]"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if status := ctx.ResponseStatus(); status != http.StatusNotFound {
		t.Fatalf("staged status = %d, want 404", status)
	}
}

// TestStatusArgumentIsCoerced pins PHP's coercion of the int parameter both
// functions take. It matters for an error page: $_SERVER holds every value as
// a string, so a page handing REDIRECT_STATUS back is handing over "404".
func TestStatusArgumentIsCoerced(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "numeric string", src: `<?php http_response_code("404");`, want: http.StatusNotFound},
		{name: "float", src: `<?php http_response_code(503.0);`, want: http.StatusServiceUnavailable},
		{name: "header code as a string", src: `<?php header("X-Reason: gone", true, "410");`, want: http.StatusGone},
		{name: "header code of zero names none", src: `<?php header("X-Reason: fine", true, 0);`, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := runner.NewContext()
			runSAPI(t, ctx, "cgi-phpscript", test.src)
			if got := ctx.ResponseStatus(); got != test.want {
				t.Fatalf("status = %d, want %d", got, test.want)
			}
		})
	}
}

// Repeated rows, checkbox arrays and `line[0][hours]` naming reach a script as
// nested arrays and not literal keys.
func TestFromRequestBracketNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		query string
		body  string
		src   string
		want  string
	}{
		{
			name:  "query nests",
			query: "a[b]=1&a[c][]=2&a[c][]=3",
			src:   `<?php echo $_GET["a"]["b"], $_GET["a"]["c"][0], $_GET["a"]["c"][1];`,
			want:  "123",
		},
		{
			name: "a repeated checkbox array keeps every value",
			body: "ids[]=7&ids[]=9&ids[]=11",
			src:  `<?php echo count($_POST["ids"]), ":", implode(",", $_POST["ids"]);`,
			want: "3:7,9,11",
		},
		{
			name: "a repeating form row",
			body: "line[0][hours]=8&line[0][note]=a&line[1][hours]=4",
			src:  `<?php echo $_POST["line"][0]["hours"], $_POST["line"][0]["note"], $_POST["line"][1]["hours"];`,
			want: "8a4",
		},
		{
			name:  "percent-encoded brackets decode before nesting",
			query: "k%5Ba%20b%5D=1",
			src:   `<?php echo $_GET["k"]["a b"];`,
			want:  "1",
		},
		{
			name:  "a name without brackets is untouched",
			query: "plain=x",
			body:  "also=y",
			src:   `<?php echo $_GET["plain"], $_POST["also"];`,
			want:  "xy",
		},
		{
			name:  "the query keeps its own order",
			query: "z=1&a=2&m=3",
			src:   `<?php echo implode(",", array_keys($_GET));`,
			want:  "z,a,m",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/submit?"+test.query, strings.NewReader(test.body))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			out, _ := runReq(t, r, test.src)
			if out != test.want {
				t.Fatalf("got %q, want %q", out, test.want)
			}
		})
	}
}

// A cookie carries bracket syntax too, and Go leaves it percent-encoded.
func TestFromRequestCookieBracketNames(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Cookie", "prefs%5Btheme%5D=dark; plain=abc")

	out, _ := runReq(t, r, `<?php echo $_COOKIE["prefs"]["theme"], ":", $_COOKIE["plain"];`)
	if want := "dark:abc"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// The decode reads a multipart body's field names the same way.
func TestFromRequestMultipartBracketNames(t *testing.T) {
	t.Parallel()
	r := multipartRequest(t, map[string]string{
		"line[0][hours]": "8",
		"line[1][hours]": "4",
	})

	out, _ := runReq(t, r, `<?php echo $_POST["line"][0]["hours"], $_POST["line"][1]["hours"];`)
	if want := "84"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// The decode drops a field nested past the limit whole; the rest still arrive.
func TestFromRequestInputLimits(t *testing.T) {
	t.Parallel()
	deep := "a" + strings.Repeat("[x]", 65) + "=1&b=kept"
	r := httptest.NewRequest("GET", "/?"+deep, nil)

	out, _ := runReq(t, r, `<?php echo isset($_GET["a"]) ? "kept" : "dropped", ":", $_GET["b"];`)
	if want := "dropped:kept"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestWithRoutePattern covers which pattern the path values come from: the
// one the route's author declared, and not the one the router matched. chi
// registers /c/{tail...} as /c/* and reports that back, so a value exported
// under a name would have no name at all with r.Pattern as the only source.
func TestWithRoutePattern(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/c/deep/path", nil)
	r.Pattern = "/c/*"
	r.SetPathValue("*", "deep/path")
	r = r.WithContext(runner.WithRoutePattern(r.Context(), "/c/{tail...}"))

	if got := runner.FromRequest(r).Path["tail"]; got != "deep/path" {
		t.Errorf("Path[tail] = %q, want %q", got, "deep/path")
	}

	// Without the declared pattern the router's own spelling is all there is,
	// and * is not a name, so there is nothing to export the value under.
	bare := httptest.NewRequest("GET", "/c/deep/path", nil)
	bare.Pattern = "/c/*"
	bare.SetPathValue("*", "deep/path")
	if got := runner.FromRequest(bare).Path; len(got) != 0 {
		t.Errorf("Path = %v with no declared pattern, want nothing", got)
	}

	// A declared pattern wins over a matched one that names the same parameter
	// differently, so one route reads the same under either router.
	mux := httptest.NewRequest("GET", "/users/42", nil)
	mux.Pattern = "GET /users/{id}"
	mux.SetPathValue("id", "42")
	mux = mux.WithContext(runner.WithRoutePattern(mux.Context(), "/users/{id}"))
	if got := runner.FromRequest(mux).Path["id"]; got != "42" {
		t.Errorf("Path[id] = %q, want 42", got)
	}
}

// TestRequestContext covers how a request-aware Go binding reaches the request:
// Register puts the Context on the runtime's context, and the binding reads it
// back off the context it holds. A context with no request answers false, which
// is what a session binding gets on a CLI run, so it reports the absence and
// reads no zero Context as an empty request.
func TestRequestContext(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/?page=2", nil)
	ctx := runner.FromRequest(r)
	rt := runner.New(io.Discard, runner.Options{})
	ctx.Register(rt)

	registered, ok := runner.RequestContext(rt.Context())
	if !ok {
		t.Fatal("RequestContext answered false on a runtime a request was registered on")
	}
	if got := registered.GetMap()["page"]; got != "2" {
		t.Errorf("the registered context reads page = %q, want 2", got)
	}

	if _, ok := runner.RequestContext(context.Background()); ok {
		t.Error("RequestContext answered true for a context carrying no request")
	}
}

// TestContext_AddResponseHeader covers the append, which is the half header()
// does not do by default. Set-Cookie is the reason it exists: a response
// setting two cookies carries the header twice, and a replace would send one.
func TestContext_AddResponseHeader(t *testing.T) {
	t.Parallel()
	ctx := runner.NewContext()
	ctx.AddResponseHeader("Set-Cookie", "a=1")
	ctx.AddResponseHeader("Set-Cookie", "b=2")

	if got := ctx.ResponseHeaders()["Set-Cookie"]; len(got) != 2 {
		t.Fatalf("Set-Cookie = %v, want both values", got)
	}
	// header() replaces unless told not to, so the two are not one call under
	// two names: what a script writes through header() drops the earlier value.
	ctx.Header("Set-Cookie: c=3")
	if got := ctx.ResponseHeaders()["Set-Cookie"]; len(got) != 1 || got[0] != "c=3" {
		t.Errorf("Set-Cookie = %v after header(), want the one value it set", got)
	}
}

// TestContext_GetAllHeaders covers the array getallheaders() answers with. A
// host-built context carries no request and answers an empty array and never
// nothing, so a script indexing the result on a CLI run reads a missing key and
// no error.
func TestContext_GetAllHeaders(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Custom", "yes")
	r.Header.Set("Content-Type", "application/json")

	headers := runner.FromRequest(r).GetAllHeaders()
	if headers == nil {
		t.Fatal("GetAllHeaders answered nothing for a request carrying headers")
	}
	for name, want := range map[string]string{"X-Custom": "yes", "Content-Type": "application/json"} {
		value, ok := headers.Get(name)
		if !ok {
			t.Errorf("GetAllHeaders holds no %s", name)
			continue
		}
		if value != want {
			t.Errorf("GetAllHeaders[%s] = %v, want %q", name, value, want)
		}
	}
	if _, ok := headers.Get("X-Not-Sent"); ok {
		t.Error("GetAllHeaders holds a header the request did not carry")
	}

	empty := runner.NewContext().GetAllHeaders()
	if empty == nil {
		t.Fatal("GetAllHeaders answered nothing for a host-built context")
	}
	if got := empty.Len(); got != 0 {
		t.Errorf("GetAllHeaders holds %d entries with no request behind it, want none", got)
	}
}

// TestContext_HTTPResponseCode covers the Go-facing call behind
// http_response_code(), which takes the SAPI as an argument. A web SAPI starts
// out answering 200; the command line starts with no status at all, so php
// answers false until something sets one and true for that first call, having
// no previous number to report.
func TestContext_HTTPResponseCode(t *testing.T) {
	t.Parallel()
	web := runner.NewContext()
	if got := web.HTTPResponseCode("http"); got != int64(http.StatusOK) {
		t.Errorf("the reporting call on a web SAPI = %v, want 200", got)
	}
	if got := web.HTTPResponseCode("http", int64(404)); got != int64(http.StatusOK) {
		t.Errorf("the first setting call on a web SAPI = %v, want the 200 it replaced", got)
	}
	if got := web.ResponseStatus(); got != http.StatusNotFound {
		t.Errorf("staged status = %d, want 404", got)
	}

	cli := runner.NewContext()
	if got := cli.HTTPResponseCode("cli"); got != false {
		t.Errorf("the reporting call with no status chosen = %v, want false", got)
	}
	if got := cli.HTTPResponseCode("cli", int64(404)); got != true {
		t.Errorf("the first setting call = %v, want true", got)
	}
	if got := cli.HTTPResponseCode("cli"); got != int64(404) {
		t.Errorf("the reporting call after one was set = %v, want 404", got)
	}

	// The parameter is an int, so php coerces the argument, and $_SERVER
	// holds every value as a string: a page handing REDIRECT_STATUS straight
	// back is handing over "503" and means 503.
	if got := cli.HTTPResponseCode("cli", "503"); got != int64(404) {
		t.Errorf("a status given as a string answered %v, want the 404 it replaced", got)
	}
	if got := cli.ResponseStatus(); got != http.StatusServiceUnavailable {
		t.Errorf("staged status = %d, want 503", got)
	}

	// A zero, a null and a false name no status, so each is the reporting call
	// and stages nothing.
	for _, arg := range []any{int64(0), nil, false} {
		if got := cli.HTTPResponseCode("cli", arg); got != int64(503) {
			t.Errorf("HTTPResponseCode(%v) = %v, want the staged 503 reported", arg, got)
		}
		if got := cli.ResponseStatus(); got != http.StatusServiceUnavailable {
			t.Errorf("HTTPResponseCode(%v) staged %d, want 503 left alone", arg, got)
		}
	}
}

func ExampleContext() {
	req := httptest.NewRequest(http.MethodGet, "/users/42?tab=profile", nil)
	req.Header.Set("X-Request-Id", "abc123")
	req.Pattern = "GET /users/{id}"
	req.SetPathValue("id", "42")

	ctx := runner.FromRequest(req)
	ctx.Header("X-Powered-By: phpscript")

	fmt.Println(ctx.GetMap()["tab"])
	fmt.Println(ctx.Path["id"])
	fmt.Println(ctx.HeaderMap()["X-Request-Id"])
	fmt.Println(ctx.ResponseHeaders().Get("X-Powered-By"))

	// Output:
	// profile
	// 42
	// abc123
	// phpscript
}
