package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
)

// browserAccept is what Chrome sends when someone follows a link. It is the one
// request shape a site's error page exists for, and the tests below use it
// verbatim rather than a tidied up "text/html" so the parser is held to a real
// header and not to an easy one.
const browserAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"

var errorPageFS = fstest.MapFS{
	"public/index.php": {Data: []byte(`<?php echo "home";`)},
	"public/404.php":   {Data: []byte(`<?php echo "sorry, " . $_SERVER["REDIRECT_URL"] . " is gone";`)},
	"public/500.php":   {Data: []byte(`<?php echo "our fault: " . $_SERVER["REDIRECT_STATUS"];`)},
	"public/503.php":   {Data: []byte(`<?php echo "back soon";`)},

	// The four shapes an endpoint can end an error on.
	"public/gone.php":     {Data: []byte(`<?php throw new Exception("the article moved", 503);`)},
	"public/broken.php":   {Data: []byte(`<?php throw new Exception("connection refused", 0);`)},
	"public/quiet.php":    {Data: []byte(`<?php http_response_code(404);`)},
	"public/payload.php":  {Data: []byte(`<?php http_response_code(404); echo '{"error":"no such user"}';`)},
	"public/declared.php": {Data: []byte(`<?php header("Content-Type: application/json"); http_response_code(404);`)},
}

func newErrorPageHandler(t *testing.T, files fstest.MapFS) *handler {
	t.Helper()
	h, err := newHandler(files, "", DefaultDocumentRoot, runner.Options{}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// fetch issues one request with the given headers and returns what the handler
// wrote.
func fetch(h http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	for name, value := range headers {
		r.Header.Set(name, value)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

// TestErrorPageAnswersBrowserNavigationOnly pins what replaces the path prefix
// the issue started from. The server mounts one catch-all, so an unrouted
// /api/... request and an unrouted /article/... one are the same request as far
// as the path goes; what tells them apart is who is asking. A person following
// a link gets the site's page and every program gets the plain status it got
// before the page existed, with nothing configured either way.
func TestErrorPageAnswersBrowserNavigationOnly(t *testing.T) {
	h := newErrorPageHandler(t, errorPageFS)

	tests := []struct {
		name    string
		method  string
		headers map[string]string
		body    string
	}{
		{
			name:    "browser navigation",
			headers: map[string]string{"Accept": browserAccept},
			body:    "sorry, /api/missing is gone",
		},
		{
			name:    "browser navigation, labelled",
			headers: map[string]string{"Accept": browserAccept, "Sec-Fetch-Dest": "document"},
			body:    "sorry, /api/missing is gone",
		},
		{
			name:    "curl",
			headers: map[string]string{"Accept": "*/*"},
			body:    "404 page not found\n",
		},
		{
			name:    "api client",
			headers: map[string]string{"Accept": "application/json"},
			body:    "404 page not found\n",
		},
		{
			// fetch() sends */* and, on a browser that labels its fetches, a
			// Sec-Fetch-Dest of empty. Either one is enough on its own.
			name:    "fetch",
			headers: map[string]string{"Accept": "*/*", "Sec-Fetch-Dest": "empty"},
			body:    "404 page not found\n",
		},
		{
			// The case Accept alone gets wrong: an XHR that asks for HTML
			// because it means to put it in the page itself.
			name:    "xhr asking for html",
			headers: map[string]string{"Accept": browserAccept, "Sec-Fetch-Dest": "empty"},
			body:    "404 page not found\n",
		},
		{
			name:    "image",
			headers: map[string]string{"Accept": "image/avif,image/webp,*/*", "Sec-Fetch-Dest": "image"},
			body:    "404 page not found\n",
		},
		{
			name:    "stylesheet",
			headers: map[string]string{"Accept": "text/css,*/*;q=0.1", "Sec-Fetch-Dest": "style"},
			body:    "404 page not found\n",
		},
		{
			name:    "no accept header",
			headers: map[string]string{},
			body:    "404 page not found\n",
		},
		{
			name:    "head request",
			method:  http.MethodHead,
			headers: map[string]string{"Accept": browserAccept},
			body:    "404 page not found\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			rr := fetch(h, method, "/api/missing", test.headers)
			if rr.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rr.Code)
			}
			if rr.Body.String() != test.body {
				t.Fatalf("body = %q, want %q", rr.Body.String(), test.body)
			}
		})
	}
}

// TestErrorPageLeavesAnAnsweredResponseAlone pins the opt-out an API needs and
// the reason it needs no prefix: an endpoint that said what its answer is keeps
// it. A payload is one way of saying so and a Content-Type is the other, and
// either works for a file under the document root or a routed endpoint, on any
// path, without the server being told which paths are the API.
func TestErrorPageLeavesAnAnsweredResponseAlone(t *testing.T) {
	h := newErrorPageHandler(t, errorPageFS)
	navigation := map[string]string{"Accept": browserAccept}

	tests := []struct {
		name        string
		path        string
		body        string
		contentType string
	}{
		{
			name: "a payload is an answer",
			path: "/payload.php",
			body: `{"error":"no such user"}`,
		},
		{
			name:        "a content type is an answer",
			path:        "/declared.php",
			body:        "",
			contentType: "application/json",
		},
		{
			name: "nothing said is not",
			path: "/quiet.php",
			body: "sorry, /quiet.php is gone",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rr := fetch(h, http.MethodGet, test.path, navigation)
			if rr.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rr.Code)
			}
			if rr.Body.String() != test.body {
				t.Fatalf("body = %q, want %q", rr.Body.String(), test.body)
			}
			if test.contentType != "" && rr.Header().Get("Content-Type") != test.contentType {
				t.Fatalf("Content-Type = %q, want %q", rr.Header().Get("Content-Type"), test.contentType)
			}
		})
	}
}

// TestUncaughtExceptionCodeBecomesTheStatus pins the mapping issue #49 asked
// for, and its limit. A code in the 4xx and 5xx range is an HTTP status and
// picks the page named after it; any other code is the script's own numbering
// and the request simply failed.
func TestUncaughtExceptionCodeBecomesTheStatus(t *testing.T) {
	h := newErrorPageHandler(t, errorPageFS)
	navigation := map[string]string{"Accept": browserAccept}

	rr := fetch(h, http.MethodGet, "/gone.php", navigation)
	if rr.Code != http.StatusServiceUnavailable || rr.Body.String() != "back soon" {
		t.Fatalf("status = %d, body = %q, want 503 and the 503 page", rr.Code, rr.Body.String())
	}

	rr = fetch(h, http.MethodGet, "/broken.php", navigation)
	if rr.Code != http.StatusInternalServerError || rr.Body.String() != "our fault: 500" {
		t.Fatalf("status = %d, body = %q, want 500 and the 500 page", rr.Code, rr.Body.String())
	}
}

// TestErrorResponseDoesNotCarryTheFailure pins that what went wrong stays in
// the log and on the trace. A message written into the body is a description of
// the site's internals handed to whoever asked for it, and a client that gets
// no page gets the status and nothing else.
func TestErrorResponseDoesNotCarryTheFailure(t *testing.T) {
	h := newErrorPageHandler(t, errorPageFS)

	rr := fetch(h, http.MethodGet, "/broken.php", map[string]string{"Accept": "application/json"})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	if rr.Body.String() != "Internal Server Error\n" {
		t.Fatalf("body = %q, want the status text alone", rr.Body.String())
	}
}

// TestErrorPageRunsOnce pins that a broken error page cannot ask for one of its
// own. A 500.php that throws is logged and the request falls back to the plain
// status, rather than dispatching a second 500.php behind it.
func TestErrorPageRunsOnce(t *testing.T) {
	h := newErrorPageHandler(t, fstest.MapFS{
		"public/broken.php": {Data: []byte(`<?php throw new Exception("connection refused", 0);`)},
		"public/500.php":    {Data: []byte(`<?php throw new Exception("the page is broken too", 0);`)},
	})

	rr := fetch(h, http.MethodGet, "/broken.php", map[string]string{"Accept": browserAccept})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	if rr.Body.String() != "Internal Server Error\n" {
		t.Fatalf("body = %q, want the status text alone", rr.Body.String())
	}
}

// TestErrorPageFallsBackThroughTheNameList pins the lookup: the status named in
// PHP, then the status named in HTML, then error.php and error.html for a site
// that keeps one page for several statuses.
func TestErrorPageFallsBackThroughTheNameList(t *testing.T) {
	navigation := map[string]string{"Accept": browserAccept}

	t.Run("static html", func(t *testing.T) {
		h := newErrorPageHandler(t, fstest.MapFS{
			"public/404.html": {Data: []byte(`<h1>gone</h1>`)},
		})
		rr := fetch(h, http.MethodGet, "/missing", navigation)
		if rr.Code != http.StatusNotFound || rr.Body.String() != "<h1>gone</h1>" {
			t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatalf("Content-Type = %q", rr.Header().Get("Content-Type"))
		}
	})

	t.Run("error.php covers what is not named", func(t *testing.T) {
		h := newErrorPageHandler(t, fstest.MapFS{
			"public/gone.php":  {Data: []byte(`<?php throw new Exception("moved", 503);`)},
			"public/error.php": {Data: []byte(`<?php echo "error " . $_SERVER["REDIRECT_STATUS"];`)},
		})
		rr := fetch(h, http.MethodGet, "/gone.php", navigation)
		if rr.Code != http.StatusServiceUnavailable || rr.Body.String() != "error 503" {
			t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
		}
	})

	t.Run("error.html covers a site with no PHP in it", func(t *testing.T) {
		h := newErrorPageHandler(t, fstest.MapFS{
			"public/index.html": {Data: []byte(`<h1>home</h1>`)},
			"public/error.html": {Data: []byte(`<h1>something went wrong</h1>`)},
		})
		rr := fetch(h, http.MethodGet, "/missing", navigation)
		if rr.Code != http.StatusNotFound || rr.Body.String() != "<h1>something went wrong</h1>" {
			t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
		}
	})

	t.Run("the status named in HTML wins over the catch-all", func(t *testing.T) {
		h := newErrorPageHandler(t, fstest.MapFS{
			"public/404.html":  {Data: []byte(`<h1>gone</h1>`)},
			"public/error.php": {Data: []byte(`<?php echo "error " . $_SERVER["REDIRECT_STATUS"];`)},
		})
		rr := fetch(h, http.MethodGet, "/missing", navigation)
		if rr.Code != http.StatusNotFound || rr.Body.String() != "<h1>gone</h1>" {
			t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
		}
	})

	t.Run("no page at all", func(t *testing.T) {
		h := newErrorPageHandler(t, fstest.MapFS{
			"public/index.php": {Data: []byte(`<?php echo "home";`)},
		})
		rr := fetch(h, http.MethodGet, "/missing", navigation)
		if rr.Code != http.StatusNotFound || rr.Body.String() != "404 page not found\n" {
			t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
		}
	})
}

// TestErrorPageSeesTheRequestItAnswersFor pins the $_SERVER keys a page is
// given. They are Apache's ErrorDocument names, so a page written against
// Apache works here, and they describe the request that failed rather than the
// page that is answering for it.
func TestErrorPageSeesTheRequestItAnswersFor(t *testing.T) {
	h := newErrorPageHandler(t, fstest.MapFS{
		"public/gone.php": {Data: []byte(`<?php throw new Exception("the article moved", 503);`)},
		"public/503.php": {Data: []byte(`<?php
echo $_SERVER["REDIRECT_STATUS"], "|",
     $_SERVER["REDIRECT_URL"], "|",
     $_SERVER["REDIRECT_QUERY_STRING"], "|",
     $_SERVER["REDIRECT_ERROR_NOTES"], "|",
     $_SERVER["SCRIPT_NAME"], "|",
     $_GET["ref"];`)},
	})

	rr := fetch(h, http.MethodGet, "/gone.php?ref=newsletter", map[string]string{"Accept": browserAccept})
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	const want = "503|/gone.php|ref=newsletter|the article moved|/503.php|newsletter"
	if rr.Body.String() != want {
		t.Fatalf("body = %q, want %q", rr.Body.String(), want)
	}
}

// TestErrorPageMayChooseItsOwnStatus pins that the page has the last word. A
// site that would rather answer a missing article with a redirect or a 410 says
// so in the page, and the status it was called for is only the default.
func TestErrorPageMayChooseItsOwnStatus(t *testing.T) {
	h := newErrorPageHandler(t, fstest.MapFS{
		"public/404.php": {Data: []byte(`<?php http_response_code(410); echo "gone for good";`)},
	})

	rr := fetch(h, http.MethodGet, "/missing", map[string]string{"Accept": browserAccept})
	if rr.Code != http.StatusGone || rr.Body.String() != "gone for good" {
		t.Fatalf("status = %d, body = %q, want 410", rr.Code, rr.Body.String())
	}
}

// TestErrorPageInAWritableDirectoryIsNotRun pins that an error page is held to
// the same rule as any other .php below the document root: a directory a
// visitor can put files in is not a directory to run code from, and a 404.php
// that arrived by upload is served as bytes rather than executed. Here the
// whole document root is writable, so the page never runs at all.
func TestErrorPageInAWritableDirectoryIsNotRun(t *testing.T) {
	h, err := newHandler(
		fstest.MapFS{"public/404.php": {Data: []byte(`<?php echo "uploaded";`)}},
		"/srv/site", DefaultDocumentRoot,
		runner.Options{WritablePaths: []string{"public"}},
		false, false,
	)
	if err != nil {
		t.Fatal(err)
	}

	rr := fetch(h, http.MethodGet, "/missing", map[string]string{"Accept": browserAccept})
	if rr.Code != http.StatusNotFound || rr.Body.String() != "404 page not found\n" {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
}

// post issues one request carrying a body and returns what the handler wrote.
func post(h http.Handler, target, contentType, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	for name, value := range headers {
		r.Header.Set(name, value)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

// TestUnroutedRequestKeepsItsBody is what lets 404.php dispatch a site's own
// routes. A request that matched no file ran no script, so nothing read its
// body and the page answering for it is the last thing that can. It sees the
// form the way the endpoint it was meant for would have.
func TestUnroutedRequestKeepsItsBody(t *testing.T) {
	h := newErrorPageHandler(t, fstest.MapFS{
		"public/404.php": {Data: []byte(`<?php
echo $_SERVER["REQUEST_METHOD"], "|",
     $_SERVER["REDIRECT_URL"], "|",
     $_POST["title"], "|",
     $_REQUEST["title"], "|",
     file_get_contents("php://input");`)},
	})

	rr := post(h, "/article/new", "application/x-www-form-urlencoded", "title=Hello", map[string]string{"Accept": browserAccept})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	const want = "POST|/article/new|Hello|Hello|title=Hello"
	if rr.Body.String() != want {
		t.Fatalf("body = %q, want %q", rr.Body.String(), want)
	}
}

// TestUnroutedPageMayAnswerOK covers the dispatcher shape end to end: the page
// reads the failed path, includes the file it routes to, and answers 200. A
// site whose 404.php is its router never wants the status it was called for.
func TestUnroutedPageMayAnswerOK(t *testing.T) {
	h := newErrorPageHandler(t, fstest.MapFS{
		"routes/article.php": {Data: []byte(`<?php echo "article ", $_POST["id"];`)},
		"public/404.php": {Data: []byte(`<?php
if ($_SERVER["REDIRECT_URL"] === "/article") {
	http_response_code(200);
	include "routes/article.php";
	return;
}
echo "no route";`)},
	})

	rr := post(h, "/article", "application/x-www-form-urlencoded", "id=42", map[string]string{"Accept": browserAccept})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if want := "article 42"; rr.Body.String() != want {
		t.Fatalf("body = %q, want %q", rr.Body.String(), want)
	}
}

// TestFailedScriptErrorPageHasNoBody pins the other half of the split. By the
// time a failing script asks for an error page it has read the body itself, so
// there is nothing left to hand on and the page is told so rather than being
// given a payload it cannot read.
func TestFailedScriptErrorPageHasNoBody(t *testing.T) {
	h := newErrorPageHandler(t, fstest.MapFS{
		"public/gone.php": {Data: []byte(`<?php throw new Exception("boom", 503);`)},
		"public/503.php": {Data: []byte(`<?php
echo $_SERVER["REQUEST_METHOD"], "|",
     count($_POST), "|",
     strlen(file_get_contents("php://input"));`)},
	})

	rr := post(h, "/gone.php", "application/x-www-form-urlencoded", "title=Hello", map[string]string{"Accept": browserAccept})
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if want := "POST|0|0"; rr.Body.String() != want {
		t.Fatalf("body = %q, want %q", rr.Body.String(), want)
	}
}

// TestUnroutedRequestKeepsItsUploads is the same rule for $_FILES. A multipart
// POST to a URL no file backs reaches the page with its parts intact, so a
// router that handles uploads can live in 404.php.
func TestUnroutedRequestKeepsItsUploads(t *testing.T) {
	h := newErrorPageHandler(t, fstest.MapFS{
		"public/404.php": {Data: []byte(`<?php
echo $_POST["title"], "|",
     $_FILES["doc"]["name"], "|",
     $_FILES["doc"]["size"], "|",
     file_get_contents($_FILES["doc"]["tmp_name"]);`)},
	})

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("title", "Notes"); err != nil {
		t.Fatal(err)
	}
	part, err := form.CreateFormFile("doc", "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	rr := post(h, "/upload", form.FormDataContentType(), body.String(), map[string]string{"Accept": browserAccept})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	if want := "Notes|notes.txt|5|hello"; rr.Body.String() != want {
		t.Fatalf("body = %q, want %q", rr.Body.String(), want)
	}
}
