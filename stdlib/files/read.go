package files

import (
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// registerReads installs the read side. Each of these looks in the runtime's
// source filesystem first, which is what an embedded or in-memory application
// ships, and falls back to the host filesystem for what only exists there, such
// as a file an earlier write produced.
func registerReads(rt *runner.Runtime, r root) {
	// glob returns the paths matching $pattern, searched in the source filesystem when one is bound, otherwise on the host; a pattern naming anything outside the root matches nothing, and a malformed one matches nothing rather than failing, as PHP's does.
	rt.RegisterFunc("glob", func(pattern string) []string {
		source := r.sourceFS()
		if source == nil {
			matches, err := filepath.Glob(r.resolve(pattern))
			if err != nil {
				return nil
			}
			return r.globSpelling(pattern, r.unresolve(matches))
		}
		name, ok := r.fsPath(pattern)
		if !ok {
			return nil
		}
		matches, err := iofs.Glob(source, name)
		if err != nil {
			return nil
		}
		return r.globSpelling(pattern, matches)
	})

	// file_get_contents returns the contents of $filename as a string, or false on failure; php://input is the raw request body (stdin under the cli SAPI), and a relative path is tried in the source filesystem first, then on the host.
	rt.RegisterFunc("file_get_contents", func(filename string) any {
		if scheme, ok := strings.CutPrefix(filename, "php://"); ok {
			if scheme == "input" {
				b, err := io.ReadAll(inputSource(rt))
				if err != nil {
					return false
				}
				return string(b)
			}
			return false
		}
		if name, ok := r.uploadPath(filename); ok {
			b, err := os.ReadFile(name)
			if err != nil {
				return false
			}
			return string(b)
		}
		if source := r.sourceFS(); source != nil {
			if name, ok := r.fsPath(filename); ok {
				if b, err := iofs.ReadFile(source, name); err == nil {
					return string(b)
				}
			}
		}
		b, err := os.ReadFile(r.resolve(filename))
		if err != nil {
			return false
		}
		return string(b)
	})
	// file_exists reports whether $filename exists, in the source filesystem or on the host.
	rt.RegisterFunc("file_exists", func(filename string) bool {
		if name, ok := r.uploadPath(filename); ok {
			_, err := os.Stat(name)
			return err == nil
		}
		if _, err := r.statSource(filename); err == nil {
			return true
		}
		_, err := os.Stat(r.resolve(filename))
		return err == nil
	})
	// filemtime returns the modification time of $filename as a Unix timestamp, or 0 when the file cannot be found; PHP returns false there.
	rt.RegisterFunc("filemtime", func(filename string) int64 {
		if name, ok := r.uploadPath(filename); ok {
			if st, err := os.Stat(name); err == nil {
				return st.ModTime().Unix()
			}
			return 0
		}
		if st, err := r.statSource(filename); err == nil {
			return st.ModTime().Unix()
		}
		if st, err := os.Stat(r.resolve(filename)); err == nil {
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
// the absence of that filesystem, or of a way to name the path inside it, as a
// miss. A miss is not an answer: every caller falls through to the host.
func (r root) statSource(p string) (iofs.FileInfo, error) {
	source := r.sourceFS()
	if source == nil {
		return nil, iofs.ErrNotExist
	}
	name, ok := r.fsPath(p)
	if !ok {
		return nil, iofs.ErrNotExist
	}
	return iofs.Stat(source, name)
}
