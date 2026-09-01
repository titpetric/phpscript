package files

import (
	"sort"

	"github.com/titpetric/phpscript/runner"
)

// registerDirs installs the directory-handle functions. They are the other way
// a script reads a directory: scandir answers with the whole listing at once,
// while opendir hands out a handle a loop walks one name at a time, which is
// what a recursive walk written in PHP looks like.
//
// The handle carries the listing taken at open time rather than an open
// descriptor. A source filesystem an embedded application ships has no
// descriptor to hold open, and both filesystems have to answer the same way, so
// the listing is read through root.readDir - the same reader scandir uses.
//
// Nothing is therefore held between opendir and closedir, and a script that
// forgets to close leaks nothing: the handle is an ordinary Go value the
// collector takes once the script drops it.
func registerDirs(rt *runner.Runtime, r root) {
	// opendir returns a handle for reading the names in $directory, or false when it is not a directory; php's $context argument is not taken, because there are no stream contexts to pass it.
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

	// readdir returns the next name in $dir_handle, or false once the listing is exhausted; "." and ".." are listed, as php lists them, and the handle is required, because there is no last-opened directory to fall back on.
	rt.RegisterFunc("readdir", func(dirHandle any) any {
		h, ok := dirHandle.(*openDir)
		if !ok || h.pos >= len(h.names) {
			return false
		}
		name := h.names[h.pos]
		h.pos++
		return name
	})

	// closedir drops the listing $dir_handle was holding and answers nothing, as php's does; a handle it closed reads as exhausted rather than raising, which is where php throws.
	rt.RegisterFunc("closedir", func(dirHandle any) {
		if h, ok := dirHandle.(*openDir); ok {
			h.names = nil
		}
	})
}

// openDir is what opendir() returns. PHP calls it a resource, a kind of value
// this runtime does not have, so the handle is a Go value the script only ever
// passes back to readdir() and closedir() - the same arrangement fopen() uses
// with its *os.File. The Go name is not dirHandle because that is the name of
// the argument the two readers take, and the generated reference is built from
// argument names.
//
// It is a pointer type because the cursor has to survive being copied into a
// PHP variable and back out again on the next readdir().
type openDir struct {
	names []string
	pos   int
}
