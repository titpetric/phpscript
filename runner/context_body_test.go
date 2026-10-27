package runner_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestRawBodyJSON pins the JSON API case: a body no form parser wants reaches
// the script through php://input, readable more than once, while $_POST stays
// empty, and invents no entries from it.
func TestRawBodyJSON(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	r := httptest.NewRequest("POST", "/echo", strings.NewReader("hours=90"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	out, _ := runReq(t, r, `<?php
echo $_POST["hours"], ";", file_get_contents("php://input");`)

	if want := "90;hours=90"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestRawBodyMultipartEmpty pins the shape PHP documents: php://input is not
// available for a multipart body and it answers empty while the form fields
// arrive through $_POST.
func TestRawBodyMultipartEmpty(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// TestPostMaxSize covers a body larger than post_max_size. PHP does not parse
// one at all and a script sees empty superglobals; the decode reports the
// reason to the host, the one place that reads it.
func TestPostMaxSize(t *testing.T) {
	t.Parallel()
	r := multipartRequest(t,
		map[string]string{"name": "bob"},
		multipartFile{field: "avatar", filename: "photo.png", content: strings.Repeat("x", 4096)},
	)
	opts := runner.Options{PostMaxSize: 1024}

	ctx := runner.FromRequestOptions(r, opts)
	defer ctx.Cleanup()

	if len(ctx.PostMap()) != 0 || len(ctx.FileMap()) != 0 {
		t.Fatalf("post = %v, files = %v, want both empty", ctx.Post, ctx.FileMap())
	}
	errs := ctx.Errors()
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want one", errs)
	}
	if !strings.Contains(errs[0].Error(), "post_max_size") {
		t.Fatalf("error = %v, want it to name post_max_size", errs[0])
	}

	// The script cannot catch it: no error reaches it, only an empty
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
// No check can judge the size before the read and the cap applies during the
// parse, and the request ends the same way an announced one does.
func TestPostMaxSizeUnknownLength(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("POST", "/submit", strings.NewReader("name="+strings.Repeat("b", 4096)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ContentLength = -1

	ctx := runner.FromRequestOptions(r, runner.Options{PostMaxSize: 1024})
	if len(ctx.PostMap()) != 0 {
		t.Fatalf("post = %v, want empty", ctx.PostMap())
	}
	if errs := ctx.Errors(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "post_max_size") {
		t.Fatalf("errors = %v, want the post_max_size error", errs)
	}
}

// runCtxOptions is runCtx for the cases that need the runtime built with
// specific options, or set up before Register puts the request onto it.
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

// Only the two form content types populate $_POST. A JSON body is
// php://input alone, as in PHP.
func TestBodyDecodeIsFormOnly(t *testing.T) {
	t.Parallel()
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

// TestContext_SetRawBody covers what php://input answers for a host that built
// its Context by hand, with no *http.Request behind it. The bytes are the body
// and no form to decode and the request fields stay as the host wrote them.
func TestContext_SetRawBody(t *testing.T) {
	t.Parallel()
	ctx := runner.NewContext()
	ctx.Post["written"] = "by the host"
	ctx.SetRawBody([]byte(`{"a":1}`))

	if got := string(ctx.RawBody()); got != `{"a":1}` {
		t.Errorf("RawBody() = %q, want the bytes that were set", got)
	}
	if got := ctx.PostMap()["written"]; got != "by the host" {
		t.Errorf("PostMap()[written] = %q, want the host's value", got)
	}
	if got := len(ctx.PostMap()); got != 1 {
		t.Errorf("PostMap() holds %d entries, want the one the host wrote", got)
	}

	// A second call replaces the body and never appends to it.
	ctx.SetRawBody([]byte("plain"))
	if got := string(ctx.RawBody()); got != "plain" {
		t.Errorf("RawBody() = %q after a second SetRawBody, want %q", got, "plain")
	}
}
