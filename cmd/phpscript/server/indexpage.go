package server

import (
	"io/fs"
	"path"
)

// indexNames are the files that answer for a directory, in the order they are
// tried. A site that has both keeps index.php: the dynamic page is the one it
// wrote to be served, and an index.html next to it is a placeholder it has not
// removed yet.
//
// index.html is here because a site is not always an application. A directory
// of hand written pages, a built front end, a documentation tree: none of them
// have an index.php to name, and before this a request for one of those
// directories answered 404 while the file sat there unread.
var indexNames = [2]string{"index.php", "index.html"}

// indexPage returns the file that answers for a directory, named relative to
// the document root, and whether the directory has one.
//
// dir is a directory below the document root, or "." for the root itself.
func (h *handler) indexPage(dir string) (string, bool) {
	for _, name := range indexNames {
		name = path.Join(dir, name)
		info, err := fs.Stat(h.public, name)
		if err == nil && !info.IsDir() {
			return name, true
		}
	}
	return "", false
}
