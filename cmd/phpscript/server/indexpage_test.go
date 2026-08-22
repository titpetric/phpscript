package server

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
)

// TestIndexPageFallsBackThroughTheNameList pins the lookup a directory is
// answered by: index.php, then index.html. A site of static files has no
// index.php to name, and before the fallback existed its front page answered
// 404 with the file sitting there unread.
func TestIndexPageFallsBackThroughTheNameList(t *testing.T) {
	tests := []struct {
		name  string
		files fstest.MapFS
		path  string
		body  string
	}{
		{
			name: "php index",
			files: fstest.MapFS{
				"public/index.php": {Data: []byte(`<?php echo "home";`)},
			},
			path: "/",
			body: "home",
		},
		{
			name: "static index",
			files: fstest.MapFS{
				"public/index.html": {Data: []byte(`<h1>home</h1>`)},
			},
			path: "/",
			body: "<h1>home</h1>",
		},
		{
			name: "php index wins over static",
			files: fstest.MapFS{
				"public/index.php":  {Data: []byte(`<?php echo "home";`)},
				"public/index.html": {Data: []byte(`<h1>placeholder</h1>`)},
			},
			path: "/",
			body: "home",
		},
		{
			name: "php index of a directory",
			files: fstest.MapFS{
				"public/docs/index.php": {Data: []byte(`<?php echo "docs at " . $_SERVER["SCRIPT_NAME"];`)},
			},
			path: "/docs/",
			body: "docs at /docs/index.php",
		},
		{
			name: "static index of a directory",
			files: fstest.MapFS{
				"public/docs/index.html": {Data: []byte(`<h1>docs</h1>`)},
			},
			path: "/docs/",
			body: "<h1>docs</h1>",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newErrorPageHandler(t, test.files)
			rr := fetch(h, http.MethodGet, test.path, nil)
			if rr.Code != http.StatusOK || rr.Body.String() != test.body {
				t.Fatalf("status = %d, body = %q, want %q", rr.Code, rr.Body.String(), test.body)
			}
			if contentType := rr.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
				t.Fatalf("Content-Type = %q, want text/html", contentType)
			}
		})
	}
}

// TestDirectoryWithoutAnIndexIsNotFound pins that a directory is answered by
// its index page and by nothing else. A listing of the files below it is not
// something a site asked to publish, and a site with a 404 page answers a
// missing index the same way it answers any other dead link.
func TestDirectoryWithoutAnIndexIsNotFound(t *testing.T) {
	h := newErrorPageHandler(t, fstest.MapFS{
		"public/index.php":        {Data: []byte(`<?php echo "home";`)},
		"public/404.html":         {Data: []byte(`<h1>gone</h1>`)},
		"public/assets/style.css": {Data: []byte(`body { color: red; }`)},
	})

	rr := fetch(h, http.MethodGet, "/assets/", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "style.css") {
		t.Fatalf("body lists the directory: %q", rr.Body.String())
	}

	rr = fetch(h, http.MethodGet, "/assets/", map[string]string{"Accept": browserAccept})
	if rr.Code != http.StatusNotFound || rr.Body.String() != "<h1>gone</h1>" {
		t.Fatalf("status = %d, body = %q, want the site's 404 page", rr.Code, rr.Body.String())
	}
}

// TestDirectoryWithoutASlashRedirects pins that the trailing slash is added
// before the index page is served. The page's relative links resolve against
// the URL it was served under, so /docs must become /docs/ or every link in it
// points a directory too high.
func TestDirectoryWithoutASlashRedirects(t *testing.T) {
	h := newErrorPageHandler(t, fstest.MapFS{
		"public/docs/index.html": {Data: []byte(`<h1>docs</h1>`)},
	})

	rr := fetch(h, http.MethodGet, "/docs?page=2", nil)
	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rr.Code)
	}
	if location := rr.Header().Get("Location"); location != "docs/?page=2" {
		t.Fatalf("Location = %q, want the same path with a slash and the query", location)
	}
}

// TestIndexPageInAWritableDirectoryIsNotRun pins that an index page is held to
// the same rule as any other .php below the document root: what a visitor put
// in a writable directory is content, and it is served as bytes rather than
// executed.
func TestIndexPageInAWritableDirectoryIsNotRun(t *testing.T) {
	h, err := newHandler(
		fstest.MapFS{"public/uploads/index.php": {Data: []byte(`<?php echo "uploaded";`)}},
		"/srv/site", DefaultDocumentRoot,
		runner.Options{WritablePaths: []string{"public/uploads"}},
		false, false,
	)
	if err != nil {
		t.Fatal(err)
	}

	rr := fetch(h, http.MethodGet, "/uploads/", nil)
	if rr.Code != http.StatusOK || rr.Body.String() != `<?php echo "uploaded";` {
		t.Fatalf("status = %d, body = %q, want the file as bytes", rr.Code, rr.Body.String())
	}
}
