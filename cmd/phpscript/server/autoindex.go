package server

import (
	"bytes"
	"html"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// autoindexStyle is the whole stylesheet, inline and small enough to send with
// every listing.
//
// titpetric/exp/cmd/indexer, whose layout this borrows, loads bootstrap from a
// CDN for the same page. A directory listing that fetches a third party asset
// tells that third party what is being browsed, and renders unstyled on a
// machine with no route out, so this one asks for nothing. There is nowhere to
// serve a separate asset from either: the document root belongs to the site.
const autoindexStyle = `
body { font: 14px/1.5 system-ui, sans-serif; margin: 2rem auto; max-width: 60rem; padding: 0 1rem; color: #222; }
h1 { font-size: 1.2rem; font-weight: 600; margin: 0 0 1rem; }
table { border-collapse: collapse; width: 100%; }
th, td { text-align: left; padding: .25rem .75rem .25rem 0; border-bottom: 1px solid #eee; white-space: nowrap; }
th { color: #666; font-weight: 500; }
td.size, td.modified { text-align: right; color: #666; font-variant-numeric: tabular-nums; }
td.name { width: 100%; white-space: normal; word-break: break-all; }
a { color: #06c; text-decoration: none; }
a:hover { text-decoration: underline; }
.thumbs { display: grid; grid-template-columns: repeat(auto-fill, minmax(9rem, 1fr)); gap: .5rem; margin-bottom: 1.5rem; }
.thumbs img { width: 100%; height: 9rem; object-fit: contain; background: #f4f4f4; border-radius: 3px; }
`

// imageExtensions are the files a listing shows instead of naming. The set is
// what a browser renders on its own; anything else is a download.
var imageExtensions = map[string]bool{
	".avif": true,
	".gif":  true,
	".jpeg": true,
	".jpg":  true,
	".png":  true,
	".svg":  true,
	".webp": true,
}

// serveAutoindex answers a directory with a listing of what is in it, and
// reports whether it did. A site that did not turn autoindex on gets a false
// answer and its 404, which is what a directory without an index page means
// when nobody asked for listings.
//
// dir names the directory relative to the document root, "." for the root
// itself. The request's own path is what the entries are linked relative to, so
// it has to be the one carrying a trailing slash; ServeHTTP redirects first.
func (h *handler) serveAutoindex(w http.ResponseWriter, r *http.Request, dir string) bool {
	if !h.autoindex {
		return false
	}
	entries, err := fs.ReadDir(h.public, dir)
	if err != nil {
		return false
	}

	page := renderAutoindex(r.URL.Path, entries)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(page)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(page)
	}
	return true
}

// renderAutoindex returns the listing page for one directory. urlPath is the
// path the directory was requested under, which every link is relative to, and
// entries are what fs.ReadDir returned: already sorted by name.
//
// The page is built here rather than with html/template. A listing is a fixed
// shape with no user supplied markup in it, so a template buys nothing but a
// parse and a reflect walk per request; the escaping it would do is done by
// escapeEntry, at the only two places a name reaches the page.
//
// Images are both shown and listed: the thumbnail is how a folder of
// screenshots is browsed, and the row below is where its size is.
func renderAutoindex(urlPath string, entries []fs.DirEntry) []byte {
	var images, dirs, files []fs.DirEntry
	for _, entry := range entries {
		switch {
		// A listing is what a site publishes on purpose, and a dotfile below
		// the document root is at best noise and at worst an .htaccess or an
		// editor's backup of a script. It is left out of the page; a request
		// that names one is still answered.
		case strings.HasPrefix(entry.Name(), "."):
		case entry.IsDir():
			dirs = append(dirs, entry)
		case imageExtensions[strings.ToLower(path.Ext(entry.Name()))]:
			images = append(images, entry)
		default:
			files = append(files, entry)
		}
	}

	title := html.EscapeString("Index of " + urlPath)

	var out bytes.Buffer
	out.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	out.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	out.WriteString("<meta name=\"robots\" content=\"noindex\">\n")
	out.WriteString("<title>" + title + "</title>\n<style>" + autoindexStyle + "</style>\n")
	out.WriteString("</head>\n<body>\n<h1>" + title + "</h1>\n")

	if len(images) > 0 {
		out.WriteString("<div class=\"thumbs\">\n")
		for _, image := range images {
			name, href := escapeEntry(image)
			out.WriteString("<a href=\"" + href + "\"><img src=\"" + href + "\" alt=\"" + name + "\" loading=\"lazy\"></a>\n")
		}
		out.WriteString("</div>\n")
	}

	out.WriteString("<table>\n<tr><th class=\"name\">Name</th><th class=\"size\">Size</th><th class=\"modified\">Modified</th></tr>\n")
	// The parent of the document root is not the site's to link to.
	if urlPath != "/" {
		out.WriteString("<tr><td class=\"name\"><a href=\"../\">../</a></td><td class=\"size\"></td><td class=\"modified\"></td></tr>\n")
	}
	for _, group := range [][]fs.DirEntry{dirs, images, files} {
		for _, entry := range group {
			writeAutoindexRow(&out, entry)
		}
	}
	out.WriteString("</table>\n</body>\n</html>\n")
	return out.Bytes()
}

// writeAutoindexRow writes one entry as a table row. An entry whose info cannot
// be read is still listed: it is there, and a size is not why anyone came.
func writeAutoindexRow(out *bytes.Buffer, entry fs.DirEntry) {
	name, href := escapeEntry(entry)

	size, modified := "", ""
	if info, err := entry.Info(); err == nil {
		if !entry.IsDir() {
			size = formatSize(info.Size())
		}
		if stamp := info.ModTime(); !stamp.IsZero() {
			modified = stamp.UTC().Format(time.DateTime)
		}
	}

	out.WriteString("<tr><td class=\"name\"><a href=\"" + href + "\">" + name + "</a></td>")
	out.WriteString("<td class=\"size\">" + size + "</td>")
	out.WriteString("<td class=\"modified\">" + modified + "</td></tr>\n")
}

// escapeEntry returns the entry's name as text in the page and as the target of
// a link to it, both escaped for where they go. A directory is named and linked
// with a trailing slash, so following it lands on that directory's own listing
// rather than on the redirect to it.
//
// The two escapings are different and both are needed: URL escaping keeps a
// name holding a space or a "?" addressable, and HTML escaping keeps one
// holding a "&" or a "<" from ending the attribute, or the document, early.
func escapeEntry(entry fs.DirEntry) (name, href string) {
	name = entry.Name()
	href = (&url.URL{Path: name}).EscapedPath()
	if entry.IsDir() {
		name += "/"
		href += "/"
	}
	return html.EscapeString(name), html.EscapeString(href)
}

// formatSize returns a file size in the largest unit that leaves it above one,
// as ls -h writes it. The exact byte count is what a HEAD request is for.
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return strconv.FormatInt(size, 10) + " B"
	}
	const scales = "KMGTPE"
	value, scale := float64(size), 0
	for value >= unit && scale < len(scales)-1 {
		value /= unit
		scale++
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + " " + string(scales[scale-1]) + "B"
}
