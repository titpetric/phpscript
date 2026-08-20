package runner_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
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
