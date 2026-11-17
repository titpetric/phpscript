package files

import (
	"sort"

	"github.com/titpetric/phpscript/runner"
)

// registerDirs installs opendir, readdir and closedir over a listing taken at open.
func registerDirs(rt *runner.Runtime, r root) {
	// opendir returns a handle for reading the names in $directory, or false
	// when $directory is not a directory; php's $context argument is not
	// taken, because there are no stream contexts to pass it.
	rt.RegisterFunc("opendir", func(directory string) any {
		names, ok := r.readDir(directory)
		if !ok {
			return false
		}
		sort.Strings(names)

		entries := make([]string, 0, len(names)+2)
		entries = append(entries, ".", "..")
		entries = append(entries, names...)
		return &openDir{names: entries}
	})

	// readdir returns the next name in $dir_handle, or false once the listing
	// runs out; it lists "." and "..", as php lists them. The handle is
	// mandatory, because there is no last-opened directory to fall back on.
	rt.RegisterFunc("readdir", func(dirHandle any) any {
		h, ok := dirHandle.(*openDir)
		if !ok || h.pos >= len(h.names) {
			return false
		}
		name := h.names[h.pos]
		h.pos++
		return name
	})

	// closedir drops the listing $dir_handle holds and answers nothing,
	// as php's does; a handle it closed reads as exhausted and not raising,
	// where php throws.
	rt.RegisterFunc("closedir", func(dirHandle any) {
		if h, ok := dirHandle.(*openDir); ok {
			h.names = nil
		}
	})
}

// openDir is the opendir handle, a pointer so its cursor survives a PHP copy.
type openDir struct {
	names []string
	pos   int
}
