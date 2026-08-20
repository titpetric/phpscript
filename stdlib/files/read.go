package files

import (
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/titpetric/phpscript/runner"
)

// registerReads installs the read side. Each of these looks in the runtime's
// source filesystem first, which is what an embedded or in-memory application
// ships, and falls back to the host filesystem for what only exists there, such
// as a file an earlier write produced.
func registerReads(rt *runner.Runtime, r root) {
	rt.RegisterFunc("glob", func(p string) ([]string, error) {
		source := r.sourceFS()
		if source == nil {
			return filepath.Glob(r.resolve(p))
		}
		return iofs.Glob(source, p)
	})

	rt.RegisterFunc("file_get_contents", func(p string) any {
		if filepath.IsAbs(p) {
			b, err := os.ReadFile(path.Clean(p))
			if err != nil {
				return false
			}
			return string(b)
		}
		if source := r.sourceFS(); source != nil {
			if b, err := iofs.ReadFile(source, r.resolve(p)); err == nil {
				return string(b)
			}
		}
		b, err := os.ReadFile(r.resolve(p))
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
		if _, err := r.statSource(p); err == nil {
			return true
		}
		_, err := os.Stat(r.resolve(p))
		return err == nil
	})
	rt.RegisterFunc("filemtime", func(p string) int64 {
		if filepath.IsAbs(p) {
			if st, err := os.Stat(path.Clean(p)); err == nil {
				return st.ModTime().Unix()
			}
		}
		if st, err := r.statSource(p); err == nil {
			return st.ModTime().Unix()
		}
		if st, err := os.Stat(r.resolve(p)); err == nil {
			return st.ModTime().Unix()
		}
		return 0
	})
}

// sourceFS is the read-only filesystem the script itself was loaded from. A
// host that runs a script straight off disk leaves it unset, so every read
// through it has to tolerate its absence.
func (r root) sourceFS() iofs.FS {
	return r.rt.FS()
}

// statSource stats a script-supplied path in the source filesystem, and reports
// the absence of that filesystem as a miss.
func (r root) statSource(p string) (iofs.FileInfo, error) {
	source := r.sourceFS()
	if source == nil {
		return nil, iofs.ErrNotExist
	}
	return iofs.Stat(source, r.resolve(p))
}
