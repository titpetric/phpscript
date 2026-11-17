package server

import (
	"io/fs"
	"path"
)

// indexNames names the files that answer for a directory, in lookup order.
var indexNames = [2]string{"index.php", "index.html"}

// indexPage returns a directory's index file relative to the document root, or false when indexNames matches nothing.
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
