package runner_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
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

// TestFromRequestRepeatedValues pins the flattening rule for a key sent more
// than once. PHP's parser assigns each repetition over the one before it, so
// the last value is the one a script sees.
func TestFromRequestRepeatedValues(t *testing.T) {
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

// TestFromRequestMultipart covers the body shape ParseForm does not decode: a
// multipart POST has to reach $_POST as well as an urlencoded one does, and its
// file parts have to arrive in $_FILES.
func TestFromRequestMultipart(t *testing.T) {
	r := multipartRequest(t,
		map[string]string{"name": "bob"},
		multipartFile{field: "avatar", filename: "photo.png", content: "binary-ish"},
	)

	ctx := runner.FromRequest(r)
	if got := ctx.Post["name"]; got != "bob" {
		t.Fatalf("$_POST[name] = %q, want %q", got, "bob")
	}

	files := ctx.Files["avatar"]
	if len(files) != 1 {
		t.Fatalf("uploads for avatar = %d, want 1", len(files))
	}
	upload := files[0]
	if upload.Error != runner.UploadErrOK {
		t.Fatalf("upload error = %d, want %d", upload.Error, runner.UploadErrOK)
	}
	if upload.Name != "photo.png" {
		t.Fatalf("upload name = %q, want %q", upload.Name, "photo.png")
	}
	if upload.Size != int64(len("binary-ish")) {
		t.Fatalf("upload size = %d, want %d", upload.Size, len("binary-ish"))
	}

	stored, err := os.ReadFile(upload.TmpName)
	if err != nil {
		t.Fatalf("read tmp_name: %v", err)
	}
	if string(stored) != "binary-ish" {
		t.Fatalf("tmp_name content = %q", stored)
	}
	if !ctx.IsUpload(upload.TmpName) {
		t.Fatalf("IsUpload(%q) = false", upload.TmpName)
	}

	// The temporary copy belongs to the request, and the host handler ends it.
	ctx.Cleanup()
	if _, err := os.Stat(upload.TmpName); !os.IsNotExist(err) {
		t.Fatalf("tmp_name after Cleanup: err = %v, want not-exist", err)
	}
}

// TestFilesSuperglobal checks the array shape a script indexes into, which is
// PHP's keys and not the Go struct behind them. The name the client sent is
// kept whole in full_path and stripped to its base in name, so a client cannot
// choose a directory for the server.
func TestFilesSuperglobal(t *testing.T) {
	r := multipartRequest(t, nil,
		multipartFile{field: "avatar", filename: `C:\Users\bob\photo.png`, content: "abc"},
	)
	ctx := runner.FromRequest(r)
	defer ctx.Cleanup()

	src := `<?php
$f = $_FILES["avatar"];
echo $f["name"] . "|" . $f["full_path"] . "|" . $f["type"] . "|" . $f["size"] . "|" . $f["error"] . "|" . $f["tmp_name"];
`
	out := runCtx(t, ctx, src)
	want := `photo.png|C:\Users\bob\photo.png|application/octet-stream|3|0|` + ctx.Files["avatar"][0].TmpName
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestFilesSuperglobalRepeatedField covers a plain field sent twice. PHP has
// nowhere to put the first file in a scalar entry, so the last one wins, the
// same way a repeated $_POST value does.
func TestFilesSuperglobalRepeatedField(t *testing.T) {
	r := multipartRequest(t, nil,
		multipartFile{field: "avatar", filename: "first.png", content: "aa"},
		multipartFile{field: "avatar", filename: "second.png", content: "bbbb"},
	)
	ctx := runner.FromRequest(r)
	defer ctx.Cleanup()

	out := runCtx(t, ctx, `<?php echo $_FILES["avatar"]["name"] . "|" . $_FILES["avatar"]["size"];`)
	if want := "second.png|4"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	// Both parts are still stored, so Cleanup removes both.
	if got := len(ctx.Files["avatar"]); got != 2 {
		t.Fatalf("uploads for avatar = %d, want 2", got)
	}
}

// TestFilesSuperglobalList covers a repeated field: PHP spells it "name[]" and
// gives the entry parallel arrays instead of scalars.
func TestFilesSuperglobalList(t *testing.T) {
	r := multipartRequest(t, nil,
		multipartFile{field: "docs[]", filename: "a.txt", content: "aa"},
		multipartFile{field: "docs[]", filename: "b.txt", content: "bbbb"},
	)
	ctx := runner.FromRequest(r)
	defer ctx.Cleanup()

	src := `<?php
$f = $_FILES["docs"];
echo $f["name"][0] . "|" . $f["name"][1] . "|" . $f["size"][0] . "|" . $f["size"][1];
`
	out := runCtx(t, ctx, src)
	want := "a.txt|b.txt|2|4"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestFromRequestUrlencodedUnaffected pins that the multipart branch is taken
// on the content type alone: an urlencoded POST still goes through ParseForm
// and reports no uploads.
func TestFromRequestUrlencodedUnaffected(t *testing.T) {
	r := httptest.NewRequest("POST", "/submit", strings.NewReader("name=bob"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx := runner.FromRequest(r)
	if got := ctx.Post["name"]; got != "bob" {
		t.Fatalf("$_POST[name] = %q", got)
	}
	if len(ctx.Files) != 0 {
		t.Fatalf("files = %v, want none", ctx.Files)
	}
}

// TestRawBodyJSON pins the JSON API case: a body no form parser wants reaches
// the script through php://input, readable more than once, while $_POST stays
// empty rather than inventing entries from it.
func TestRawBodyJSON(t *testing.T) {
	r := httptest.NewRequest("PUT", "/api/thing", strings.NewReader(`{"hours":90}`))
	r.Header.Set("Content-Type", "application/json")

	out, _ := runReq(t, r, `<?php
echo file_get_contents("php://input"), ";";
$decoded = json_decode(file_get_contents("php://input"), true);
echo $decoded["hours"], ";";
echo count($_POST), ";";
$h = fopen("php://input", "r");
echo stream_get_contents($h), ";";
fclose($h);
echo file_get_contents("php://input");`)

	want := `{"hours":90};90;0;{"hours":90};{"hours":90}`
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestRawBodyFormEncoded pins that buffering the body does not cost the form
// parse: $_POST decodes as before, and php://input still answers with the raw
// urlencoded bytes, as PHP's does.
func TestRawBodyFormEncoded(t *testing.T) {
	r := httptest.NewRequest("POST", "/echo", strings.NewReader("hours=90"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	out, _ := runReq(t, r, `<?php
echo $_POST["hours"], ";", file_get_contents("php://input");`)

	if want := "90;hours=90"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestRawBodyMultipartEmpty pins the shape PHP documents: php://input is not
// available for a multipart body, so it answers empty while the form fields
// arrive through $_POST.
func TestRawBodyMultipartEmpty(t *testing.T) {
	r := multipartRequest(t, map[string]string{"name": "bob"})

	out, _ := runReq(t, r, `<?php
echo $_POST["name"], ";", strlen(file_get_contents("php://input"));`)

	if want := "bob;0"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestRawBodyOverLimitIsDropped pins the size cap: a body past post_max_size
// leaves php://input empty along with the superglobals, and the reason goes to
// the host.
func TestRawBodyOverLimitIsDropped(t *testing.T) {
	r := httptest.NewRequest("POST", "/submit", strings.NewReader(`{"payload":"`+strings.Repeat("x", 4096)+`"}`))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = -1

	ctx := runner.FromRequestOptions(r, runner.Options{PostMaxSize: 1024})
	if got := ctx.RawBody(); len(got) != 0 {
		t.Fatalf("raw body = %d bytes, want none", len(got))
	}
	if errs := ctx.Errors(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "post_max_size") {
		t.Fatalf("errors = %v, want the post_max_size error", errs)
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

// wantServer checks the $_SERVER keys a request produced. A want value of ""
// asserts the key is absent, which is how PHP says "not this kind of request":
// no HTTPS on a plain one, no CONTENT_LENGTH on a chunked one.
func wantServer(t *testing.T, server map[string]string, want map[string]string) {
	t.Helper()
	for key, value := range want {
		got, ok := server[key]
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
	r := httptest.NewRequest("GET", "/index.php?a=1&b=two", nil)
	r.RemoteAddr = "127.0.0.1:56138"

	ctx := runner.FromRequest(r)
	wantServer(t, ctx.Server, map[string]string{
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
// keys PHP has for it. An IPv6 peer arrives bracketed and is written the way
// PHP writes it, without brackets.
func TestServerVarsRemoteAddr(t *testing.T) {
	cases := []struct {
		remote string
		addr   string
		port   string
	}{
		{remote: "127.0.0.1:56138", addr: "127.0.0.1", port: "56138"},
		{remote: "[::1]:37154", addr: "::1", port: "37154"},
		// A Go host may put anything in the field; one without a port is kept
		// whole rather than guessed at.
		{remote: "unix-socket", addr: "unix-socket", port: ""},
		{remote: "", addr: "", port: ""},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.remote
		ctx := runner.FromRequest(r)
		wantServer(t, ctx.Server, map[string]string{
			"REMOTE_ADDR": tc.addr,
			"REMOTE_PORT": tc.port,
		})
	}
}

// TestServerVarsContentHeaders covers the two keys that describe a body. PHP
// mirrors the headers rather than the body, so a GET that sent a content type
// has CONTENT_TYPE, and a body of zero bytes that announced its length has
// CONTENT_LENGTH "0".
func TestServerVarsContentHeaders(t *testing.T) {
	r := httptest.NewRequest("POST", "/submit", strings.NewReader("name=bob&x=1"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx := runner.FromRequest(r)
	wantServer(t, ctx.Server, map[string]string{
		"REQUEST_METHOD": "POST",
		"CONTENT_TYPE":   "application/x-www-form-urlencoded",
		"CONTENT_LENGTH": "12",
		// PHP mirrors every header into HTTP_*, these two included.
		"HTTP_CONTENT_TYPE": "application/x-www-form-urlencoded",
	})

	// A content type without a body still reaches CONTENT_TYPE.
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Content-Type", "text/plain")
	wantServer(t, runner.FromRequest(r).Server, map[string]string{
		"CONTENT_TYPE":   "text/plain",
		"CONTENT_LENGTH": "",
	})

	// An announced empty body has a length; PHP writes the "0".
	r = httptest.NewRequest("POST", "/", strings.NewReader(""))
	r.Header.Set("Content-Type", "text/plain")
	r.Header.Set("Content-Length", "0")
	wantServer(t, runner.FromRequest(r).Server, map[string]string{
		"CONTENT_LENGTH": "0",
	})

	// A chunked body announces no length, and PHP leaves the key out.
	r = httptest.NewRequest("POST", "/", strings.NewReader("hello"))
	r.Header.Set("Content-Type", "text/plain")
	r.ContentLength = -1
	wantServer(t, runner.FromRequest(r).Server, map[string]string{
		"CONTENT_TYPE":   "text/plain",
		"CONTENT_LENGTH": "",
	})
}

// TestServerVarsTLS covers a request this process accepted over TLS. PHP sets
// HTTPS only then, and to "on"; an X-Forwarded-Proto is the client's word and
// changes neither key.
func TestServerVarsTLS(t *testing.T) {
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

	wantServer(t, runner.FromRequest(got).Server, map[string]string{
		"REQUEST_SCHEME": "https",
		"HTTPS":          "on",
	})

	plain := httptest.NewRequest("GET", "/", nil)
	plain.Header.Set("X-Forwarded-Proto", "https")
	wantServer(t, runner.FromRequest(plain).Server, map[string]string{
		"REQUEST_SCHEME": "http",
		"HTTPS":          "",
		// The header is still readable, as any other header is.
		"HTTP_X_FORWARDED_PROTO": "https",
	})
}

// TestServerVarsRequestTime covers the request start time. The seconds key is
// the whole second of the float one, and PHP types the first as an integer and
// the second as a float rather than as the strings every other key holds.
func TestServerVarsRequestTime(t *testing.T) {
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

// TestServerSuperglobalStrings pins that every other key reaches a script as a
// string, which is what $_SERVER holds.
func TestServerSuperglobalStrings(t *testing.T) {
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

func TestLocationHeaderSetsRedirectStatus(t *testing.T) {
	ctx := runner.NewContext()
	rt := runner.New(&strings.Builder{}, runner.Options{})
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
// "HTTP/1.0 404 Not Found" carries no name and no value, only a status, and it
// used to be dropped on the floor for want of a name:value shape.
func TestStatusLineHeaderSetsStatus(t *testing.T) {
	tests := []struct {
		header string
		want   int
	}{
		{header: `HTTP/1.0 404 Not Found`, want: http.StatusNotFound},
		{header: `HTTP/1.1 503 Service Unavailable`, want: http.StatusServiceUnavailable},
		{header: `HTTP/1.1 204`, want: http.StatusNoContent},

		// Not status lines. The first is a header named Status, the rest name
		// no status this response could be sent with.
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
// one to hand back. On the command line a script starts with none, which is
// what the false and the true are: nothing to report, and no previous status to
// return for the first one set.
//
// json_encode keeps the types visible; phpscript has no var_dump.
func TestHTTPResponseCode(t *testing.T) {
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
// and leaves the response where it was.
func TestHTTPResponseCodeIgnoresANonStatus(t *testing.T) {
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

// TestPostMaxSize covers a body larger than post_max_size. PHP does not parse
// one at all, so a script sees empty superglobals; the reason is reported to
// the host, which is the only place it can be seen.
func TestPostMaxSize(t *testing.T) {
	r := multipartRequest(t,
		map[string]string{"name": "bob"},
		multipartFile{field: "avatar", filename: "photo.png", content: strings.Repeat("x", 4096)},
	)
	opts := runner.Options{PostMaxSize: 1024}

	ctx := runner.FromRequestOptions(r, opts)
	defer ctx.Cleanup()

	if len(ctx.Post) != 0 || len(ctx.Files) != 0 {
		t.Fatalf("post = %v, files = %v, want both empty", ctx.Post, ctx.Files)
	}
	errs := ctx.Errors()
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want one", errs)
	}
	if !strings.Contains(errs[0].Error(), "post_max_size") {
		t.Fatalf("error = %v, want it to name post_max_size", errs[0])
	}

	// The script cannot catch it: there is nothing to catch, only an empty
	// superglobal. The host gets it through the runtime instead.
	var recorded []error
	out := runCtxOptions(t, ctx, opts, func(rt *runner.Runtime) {
		rt.OnError(func(err error) { recorded = append(recorded, err) })
	}, `<?php
try {
	echo isset($_POST["name"]) ? "has-name" : "no-name";
	echo isset($_FILES["avatar"]) ? "|has-file" : "|no-file";
} catch (Exception $e) {
	echo "|caught";
}`)
	if want := "no-name|no-file"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	if len(recorded) != 1 || !strings.Contains(recorded[0].Error(), "post_max_size") {
		t.Fatalf("recorded = %v, want the post_max_size error", recorded)
	}
}

// TestPostMaxSizeUnknownLength covers a body that does not announce its length.
// It cannot be judged before it is read, so it is capped as it is parsed, and
// ends the same way an announced one does.
func TestPostMaxSizeUnknownLength(t *testing.T) {
	r := httptest.NewRequest("POST", "/submit", strings.NewReader("name="+strings.Repeat("b", 4096)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ContentLength = -1

	ctx := runner.FromRequestOptions(r, runner.Options{PostMaxSize: 1024})
	if len(ctx.Post) != 0 {
		t.Fatalf("post = %v, want empty", ctx.Post)
	}
	if errs := ctx.Errors(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "post_max_size") {
		t.Fatalf("errors = %v, want the post_max_size error", errs)
	}
}

// TestUploadMaxFilesize covers a single file part over the limit. PHP keeps the
// rest of the form and marks that one entry UPLOAD_ERR_INI_SIZE, so a script
// can tell the user which file was refused.
func TestUploadMaxFilesize(t *testing.T) {
	r := multipartRequest(t,
		map[string]string{"name": "bob"},
		multipartFile{field: "small", filename: "small.txt", content: "tiny"},
		multipartFile{field: "big", filename: "big.txt", content: strings.Repeat("x", 4096)},
	)

	ctx := runner.FromRequestOptions(r, runner.Options{UploadMaxFilesize: 1024})
	defer ctx.Cleanup()

	if got := ctx.Post["name"]; got != "bob" {
		t.Fatalf("$_POST[name] = %q, want the form to survive", got)
	}
	small := ctx.Files["small"][0]
	if small.Error != runner.UploadErrOK || small.TmpName == "" {
		t.Fatalf("small upload = %+v, want it stored", small)
	}
	// PHP describes a refused part by the names the client sent and nothing
	// else: no type, no size, no temporary file.
	big := ctx.Files["big"][0]
	if big.Error != runner.UploadErrIniSize {
		t.Fatalf("big upload error = %d, want %d", big.Error, runner.UploadErrIniSize)
	}
	if big.Name != "big.txt" || big.TmpName != "" || big.Type != "" || big.Size != 0 {
		t.Fatalf("big upload = %+v, want the names only", big)
	}
	if errs := ctx.Errors(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "upload_max_filesize") {
		t.Fatalf("errors = %v, want the upload_max_filesize error", errs)
	}

	out := runCtx(t, ctx, `<?php
echo $_FILES["small"]["error"] === UPLOAD_ERR_OK ? "small-ok" : "small-refused";
echo $_FILES["big"]["error"] === UPLOAD_ERR_INI_SIZE ? "|big-too-large" : "|big-ok";`)
	if want := "small-ok|big-too-large"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// runCtxOptions is runCtx for the cases that need the runtime built with
// specific options, or configured before the request is registered onto it.
func runCtxOptions(t *testing.T, ctx runner.Context, opts runner.Options, setup func(*runner.Runtime), src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, opts)
	stdlib.Register(rt)
	if setup != nil {
		setup(rt)
	}
	ctx.Register(rt)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// Repeated rows, checkbox arrays and `line[0][hours]` naming reach a script as
// nested arrays rather than literal keys.
func TestFromRequestBracketNames(t *testing.T) {
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
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Cookie", "prefs%5Btheme%5D=dark; plain=abc")

	out, _ := runReq(t, r, `<?php echo $_COOKIE["prefs"]["theme"], ":", $_COOKIE["plain"];`)
	if want := "dark:abc"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// A multipart body's field names are read the same way.
func TestFromRequestMultipartBracketNames(t *testing.T) {
	r := multipartRequest(t, map[string]string{
		"line[0][hours]": "8",
		"line[1][hours]": "4",
	})

	out, _ := runReq(t, r, `<?php echo $_POST["line"][0]["hours"], $_POST["line"][1]["hours"];`)
	if want := "84"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// A field nested past the limit is dropped whole; the rest still arrive.
func TestFromRequestInputLimits(t *testing.T) {
	deep := "a" + strings.Repeat("[x]", 65) + "=1&b=kept"
	r := httptest.NewRequest("GET", "/?"+deep, nil)

	out, _ := runReq(t, r, `<?php echo isset($_GET["a"]) ? "kept" : "dropped", ":", $_GET["b"];`)
	if want := "dropped:kept"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// A hand-assembled Context has no request to decode, so its flat maps are
// promoted as they stand: the fixture harness, a CLI run, an embedder.
func TestRegisterFallsBackToFlatMaps(t *testing.T) {
	c := runner.NewContext()
	c.Get["greeting"] = "hello"
	c.Post["message"] = "form"
	c.Cookie["session"] = "abc"

	out := runCtx(t, c, `<?php echo $_GET["greeting"], $_POST["message"], $_COOKIE["session"];`)
	if want := "helloformabc"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// Only the two form content types populate $_POST. A JSON body is
// php://input and nothing else, as it is in PHP.
func TestBodyDecodeIsFormOnly(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        string
	}{
		{"urlencoded", "application/x-www-form-urlencoded", "1"},
		{"json", "application/json", "0"},
		{"none", "", "0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api", strings.NewReader("hours=90"))
			if test.contentType != "" {
				r.Header.Set("Content-Type", test.contentType)
			}
			if out, _ := runReq(t, r, `<?php echo count($_POST);`); out != test.want {
				t.Fatalf("got %q, want %q", out, test.want)
			}
		})
	}
}
