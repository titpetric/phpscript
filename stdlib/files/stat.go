package files

import (
	iofs "io/fs"
	"os"
	stdpath "path"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// registerStat installs the questions a script asks about a path: what kind of
// thing is there, how big it is, and what a directory holds.
//
// They live here rather than beside the other new bindings because they have to
// resolve a path exactly as file_get_contents resolves it - through the bound
// root, the per-request working directory chdir() moved, and the source
// filesystem before the host. A second resolver written elsewhere would be a
// second answer to "which file is that", which is the one thing a sandbox
// cannot have two of.
func registerStat(rt *runner.Runtime, r root) {
	// is_file reports whether $filename names an ordinary file, in the source filesystem or on the host; a directory is not one, and neither is a path that does not exist.
	rt.RegisterFunc("is_file", func(filename string) bool {
		info, ok := r.stat(filename)
		return ok && info.Mode().IsRegular()
	})

	// is_dir reports whether $filename names a directory.
	rt.RegisterFunc("is_dir", func(filename string) bool {
		info, ok := r.stat(filename)
		return ok && info.IsDir()
	})

	// is_readable reports whether $filename can be opened for reading, which here is whether it exists: a path inside the root is readable by the process that is asking, and the source filesystem carries no permissions of its own.
	rt.RegisterFunc("is_readable", func(filename string) bool {
		_, ok := r.stat(filename)
		return ok
	})

	// is_writable reports whether $filename could be written, which asks writable_paths rather than the file mode; a path the allowlist refuses is not writable however the host's permissions read, and a path it allows is, whether or not the file exists yet.
	rt.RegisterFunc("is_writable", func(filename string) bool {
		_, err := r.resolveWrite("is_writable", filename)
		return err == nil
	})

	// is_executable reports whether $filename has an execute bit set on the host; a file that exists only in the source filesystem is not executable, because there is nothing there to run.
	rt.RegisterFunc("is_executable", func(filename string) bool {
		info, err := os.Stat(r.resolve(filename))
		return err == nil && info.Mode().Perm()&0o111 != 0
	})

	// filesize returns the size of $filename in bytes, or false when it cannot be found.
	rt.RegisterFunc("filesize", func(filename string) any {
		info, ok := r.stat(filename)
		if !ok {
			return false
		}
		return info.Size()
	})

	// filetype returns "file", "dir" or "link" for what $filename names, or false when it cannot be found; the exotic types php can answer with - fifo, block, char, socket - are reported as "file", because nothing inside a source filesystem can be one.
	rt.RegisterFunc("filetype", func(filename string) any {
		info, ok := r.stat(filename)
		if !ok {
			return false
		}
		switch {
		case info.IsDir():
			return "dir"
		case info.Mode()&iofs.ModeSymlink != 0:
			return "link"
		}
		return "file"
	})

	// realpath returns $path written from the root, with . and .. resolved, or false when nothing is there; php answers a host path, and a runtime whose scripts may be served out of an embedded tree has none to answer with, so this answers the spelling getcwd() and __DIR__ use.
	rt.RegisterFunc("realpath", func(path string) any {
		if _, ok := r.stat(path); !ok {
			return false
		}
		name, ok := r.fsPath(path)
		if !ok {
			return false
		}
		return "/" + strings.TrimPrefix(name, "/")
	})

	// scandir returns the names in the directory $directory, sorted, with "." and ".." first as php lists them; false when $directory is not a directory.
	rt.RegisterFunc("scandir", func(directory string) any {
		names, ok := r.readDir(directory)
		if !ok {
			return false
		}
		sort.Strings(names)

		out := make([]any, 0, len(names)+2)
		out = append(out, ".", "..")
		for _, name := range names {
			out = append(out, name)
		}
		return out
	})

	// pathinfo returns the parts of $path as an array of dirname, basename, extension and filename; a path with no dot carries no extension key, which is what php leaves out rather than answering empty. It never touches the filesystem, so it answers about a path that does not exist.
	rt.RegisterFunc("pathinfo", func(path string) *model.Array {
		out := model.NewArraySize(4)
		out.Set("dirname", stdpath.Dir(strings.TrimRight(path, "/")))

		base := stdpath.Base(strings.TrimRight(path, "/"))
		out.Set("basename", base)

		if dot := strings.LastIndex(base, "."); dot > 0 {
			out.Set("extension", base[dot+1:])
			out.Set("filename", base[:dot])
			return out
		}
		out.Set("filename", base)
		return out
	})
}

// stat answers about a script-supplied path the way the read side answers about
// one: an uploaded file first, then the source filesystem, then the host. The
// order is what makes is_file() agree with file_get_contents() about which file
// a name reaches.
func (r root) stat(p string) (iofs.FileInfo, bool) {
	if name, ok := r.uploadPath(p); ok {
		info, err := os.Stat(name)
		return info, err == nil
	}
	if info, err := r.statSource(p); err == nil {
		return info, true
	}
	info, err := os.Stat(r.resolve(p))
	return info, err == nil
}

// readDir lists a directory through the same order, and reports "not a
// directory" the same way for both filesystems.
func (r root) readDir(p string) ([]string, bool) {
	if source := r.sourceFS(); source != nil {
		if name, ok := r.fsPath(p); ok {
			if entries, err := iofs.ReadDir(source, strings.TrimPrefix(name, "/")); err == nil {
				return entryNames(entries), true
			}
		}
	}

	entries, err := os.ReadDir(r.resolve(p))
	if err != nil {
		return nil, false
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, true
}

// entryNames is the same reduction for the source filesystem's entry type.
func entryNames(entries []iofs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
