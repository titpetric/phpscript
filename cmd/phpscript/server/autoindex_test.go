package server

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
)

var autoindexFS = fstest.MapFS{
	"public/index.php":             {Data: []byte(`<?php echo "home";`)},
	"public/files/notes.txt":       {Data: []byte(strings.Repeat("x", 2048))},
	"public/files/report & q.pdf":  {Data: []byte(`%PDF`)},
	"public/files/photo.PNG":       {Data: []byte(`png`)},
	"public/files/.hidden":         {Data: []byte(`secret`)},
	"public/files/archive/old.txt": {Data: []byte(`old`)},
}

// newAutoindexHandler builds a handler with listings turned on, which is what
// `autoindex: true` in the configuration reaches.
func newAutoindexHandler(t *testing.T, files fstest.MapFS) *handler {
	t.Helper()
	h, err := newHandler(files, "", DefaultDocumentRoot, runner.Options{}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestAutoindexIsOffUnlessTheSiteAsksForIt pins the default. Publishing every
// file below the document root is a decision, so a directory with no index page
// is a 404 until the configuration says otherwise.
func TestAutoindexIsOffUnlessTheSiteAsksForIt(t *testing.T) {
	rr := fetch(newErrorPageHandler(t, autoindexFS), http.MethodGet, "/files/", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}

	rr = fetch(newAutoindexHandler(t, autoindexFS), http.MethodGet, "/files/", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want the listing", rr.Code)
	}
	if contentType := rr.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", contentType)
	}
}

// TestAutoindexListsTheDirectory pins what a listing holds: every entry linked
// relative to the directory it was requested under, images shown as well as
// named, sizes for files, a way back up, and nothing beginning with a dot.
func TestAutoindexListsTheDirectory(t *testing.T) {
	rr := fetch(newAutoindexHandler(t, autoindexFS), http.MethodGet, "/files/", nil)
	body := rr.Body.String()

	for _, want := range []string{
		`<title>Index of /files/</title>`,
		`<a href="archive/">archive/</a>`,
		`<a href="notes.txt">notes.txt</a>`,
		`2.0 KB`,
		`<img src="photo.PNG"`,
		`<a href="../">../</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("listing does not contain %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, ".hidden") {
		t.Errorf("listing shows a dotfile:\n%s", body)
	}
	// Nothing is fetched from anywhere else: a listing that loaded a stylesheet
	// from a CDN would tell that CDN what is being browsed.
	if strings.Contains(body, "://") {
		t.Errorf("listing names an off site URL:\n%s", body)
	}
}

// TestAutoindexEscapesNames pins both escapings a name goes through. A file
// named with an "&" has to stay addressable as a link and must not end the
// attribute it sits in.
func TestAutoindexEscapesNames(t *testing.T) {
	body := fetch(newAutoindexHandler(t, autoindexFS), http.MethodGet, "/files/", nil).Body.String()
	if !strings.Contains(body, `<a href="report%20&amp;%20q.pdf">report &amp; q.pdf</a>`) {
		t.Fatalf("name is not escaped for both places it goes:\n%s", body)
	}
}

// TestAutoindexAnswersHeadWithHeadersAlone pins that a HEAD gets the size of
// the listing without the listing, which is what a HEAD is for.
func TestAutoindexAnswersHeadWithHeadersAlone(t *testing.T) {
	h := newAutoindexHandler(t, autoindexFS)
	get := fetch(h, http.MethodGet, "/files/", nil)
	head := fetch(h, http.MethodHead, "/files/", nil)

	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("status = %d, body = %d bytes, want an empty 200", head.Code, head.Body.Len())
	}
	if length := head.Header().Get("Content-Length"); length != strconv.Itoa(get.Body.Len()) {
		t.Fatalf("Content-Length = %q, want %d", length, get.Body.Len())
	}
}

// TestAutoindexDoesNotReplaceAnIndexPage pins the order: a directory with an
// index page is answered by it, and the listing is only for directories that
// have none.
func TestAutoindexDoesNotReplaceAnIndexPage(t *testing.T) {
	h := newAutoindexHandler(t, autoindexFS)
	rr := fetch(h, http.MethodGet, "/", nil)
	if rr.Code != http.StatusOK || rr.Body.String() != "home" {
		t.Fatalf("status = %d, body = %q, want the index page", rr.Code, rr.Body.String())
	}
}

// TestAutoindexOfTheDocumentRoot pins that the root listing offers no way above
// itself. The parent of the document root is not the site's to link to.
func TestAutoindexOfTheDocumentRoot(t *testing.T) {
	h := newAutoindexHandler(t, fstest.MapFS{
		"public/notes.txt": {Data: []byte(`notes`)},
	})
	rr := fetch(h, http.MethodGet, "/", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want the listing", rr.Code)
	}
	if strings.Contains(rr.Body.String(), `href="../"`) {
		t.Fatalf("root listing links above the document root:\n%s", rr.Body.String())
	}
}

func TestFormatSize(t *testing.T) {
	for _, test := range []struct {
		size int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{5 * 1024 * 1024 * 1024, "5.0 GB"},
	} {
		if got := formatSize(test.size); got != test.want {
			t.Errorf("formatSize(%d) = %q, want %q", test.size, got, test.want)
		}
	}
}
