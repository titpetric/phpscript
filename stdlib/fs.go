package stdlib

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// RegisterFS installs filesystem IO shims rooted at dir. Reads and writes are
// confined to that directory tree, mirroring how the upstream engine runs inside
// its project checkout. Paths from PHP are treated as relative to dir.
//
// Go's fs.FS is read-only, so the engine's writes (fopen/fwrite/mkdir) use the
// os package against the same root; the runner's include resolution still uses
// the fs.FS abstraction passed to runner.New.
func RegisterFS(rt *runner.Runtime, dir string) {
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return path.Clean(p)
		}
		clean := path.Clean("/" + filepath.ToSlash(p))
		return filepath.Join(dir, filepath.FromSlash(clean))
	}

	openMode := func(name, mode string) (*os.File, error) {
		switch strings.TrimSuffix(mode, "b") {
		case "r", "r+":
			return os.OpenFile(name, os.O_RDWR, 0)
		case "a", "a+":
			return os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
		default: // "w", "w+"
			return os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
		}
	}

	rt.RegisterFunc("glob", func(p string) ([]string, error) {
		return fs.Glob(rt.FS(), p)
	})

	// file_get_contents: prefer the runner's FS, fall back to OS for absolute paths or missing files
	rt.RegisterFunc("file_get_contents", func(p string) any {
		if filepath.IsAbs(p) {
			b, err := os.ReadFile(path.Clean(p))
			if err != nil {
				return false
			}
			return string(b)
		}
		// try runner's FS first
		b, err := fs.ReadFile(rt.FS(), resolve(p))
		if err == nil {
			return string(b)
		}
		// fallback to OS read (outside root)
		b, err = os.ReadFile(resolve(p))
		if err != nil {
			return false
		}
		return string(b)
	})
	rt.RegisterFunc("file_exists", func(p string) bool {
		if filepath.IsAbs(p) {
			_, err := os.Stat(path.Clean(p))
			return err == nil
		}
		// check in runner FS
		_, err := fs.Stat(rt.FS(), resolve(p))
		if err == nil {
			return true
		}
		// fallback to OS stat
		_, err = os.Stat(resolve(p))
		return err == nil
	})
	rt.RegisterFunc("filemtime", func(p string) int64 {
		if filepath.IsAbs(p) {
			if st, err := os.Stat(path.Clean(p)); err == nil {
				return st.ModTime().Unix()
			}
		}
		// check in runner FS
		if st, err := fs.Stat(rt.FS(), resolve(p)); err == nil {
			return st.ModTime().Unix()
		}
		// fallback to OS stat
		if st, err := os.Stat(resolve(p)); err == nil {
			return st.ModTime().Unix()
		}
		return 0
	})
	rt.RegisterFunc("dirname", func(p string) string {
		d := path.Dir(strings.TrimRight(filepath.ToSlash(p), "/"))
		return d
	})
	rt.RegisterFunc("basename", func(p string) string {
		return path.Base(filepath.ToSlash(p))
	})
	rt.RegisterFunc("mkdir", func(p string, _ ...any) bool {
		return os.MkdirAll(resolve(p), 0o755) == nil
	})
	rt.RegisterFunc("unlink", func(p string) bool {
		return os.Remove(resolve(p)) == nil
	})
	rt.RegisterFunc("touch", func(p string, mtime ...int64) bool {
		name := resolve(p)
		f, err := os.OpenFile(name, os.O_CREATE, 0o644)
		if err != nil {
			return false
		}
		if err := f.Close(); err != nil {
			return false
		}

		t := time.Now()
		if len(mtime) > 0 {
			t = time.Unix(mtime[0], 0)
		}
		return os.Chtimes(name, t, t) == nil
	})

	rt.RegisterFunc("fopen", func(p, mode string) any {
		f, err := openMode(resolve(p), mode)
		if err != nil {
			return false
		}
		return f
	})
	// fwrite takes any writer, not just a handle fopen() produced: STDOUT and
	// STDERR are the process's own streams, and a binding is free to hand PHP
	// an io.Writer of its own.
	rt.RegisterFunc("fwrite", func(stream io.Writer, s string) any {
		if stream == nil {
			return false
		}
		n, err := io.WriteString(stream, s)
		if err != nil {
			return false
		}
		return int64(n)
	})
	rt.RegisterFunc("fclose", func(f *os.File) bool {
		if f == nil {
			return false
		}
		return f.Close() == nil
	})
}
