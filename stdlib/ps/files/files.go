// Package files holds the filesystem shims phpscript exposes to PHP: path
// helpers, reads, writes, streams, and the uploaded-file functions that go with
// $_FILES. They sit here, not in stdlib, because they are the only
// standard-library part bound to a host directory. A host that runs untrusted
// scripts may leave them out.
//
// Go's fs.FS only reads and writes (fopen/fwrite/mkdir) use the os package
// against the same root; the runner's include resolution still uses the fs.FS
// abstraction passed to runner.New.
package files

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the filesystem bindings rooted at the process working directory.
func init() {
	runner.RegisterArea(`Runtime\PS\Files`, Register)
}

// Register installs the filesystem shims rooted at the working directory.
func Register(rt *runner.Runtime) {
	RegisterRoot(rt, ".")
}

// RegisterRoot installs the filesystem shims rooted at dir, replacing whatever
// root stands before it; docs/security.md argues the boundary.
func RegisterRoot(rt *runner.Runtime, dir string) {
	r := root{rt: rt, dir: dir}
	r.writable = WritableRoots(dir, rt.WritablePaths())

	registerPaths(rt)
	registerWorkDir(rt)
	registerReads(rt, r)
	registerStat(rt, r)
	registerDirs(rt, r)
	registerStreamReads(rt)
	registerWrites(rt, r)
	registerStreams(rt, r)
	registerCSV(rt)
	registerUploads(rt, r)
}

// root is the bound directory and the runtime the shims read through.
type root struct {
	rt  *runner.Runtime
	dir string

	// writable is the resolved writable_paths allowlist. An empty list means
	// no restriction. A configuration that names none gets that.
	writable []string
}

// resolve maps a script path onto a host path inside the root and refuses one that climbs out.
func (r root) resolve(p string) string {
	rel, _ := r.fsPath(p)
	return filepath.Join(r.dir, filepath.FromSlash(rel))
}

// fsPath names the same file resolve does as a slash path, false only for an empty path.
func (r root) fsPath(p string) (string, bool) {
	if p == "" {
		return "", false
	}
	return r.rt.ResolvePath(p), true
}

// uploadPath reports a host path this request's upload registry issued, refusing any other.
func (r root) uploadPath(p string) (string, bool) {
	if !filepath.IsAbs(p) {
		return "", false
	}
	request, ok := runner.RequestContext(r.rt.Context())
	if !ok || !request.IsUpload(p) {
		return "", false
	}
	return path.Clean(filepath.ToSlash(p)), true
}

// unresolve undoes resolve over host paths, naming them from the source root.
func (r root) unresolve(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	prefix := filepath.Clean(r.dir) + string(filepath.Separator)
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, filepath.ToSlash(strings.TrimPrefix(name, prefix)))
	}
	return out
}

// globSpelling rewrites matches into the pattern's own shape, root-absolute or working-directory-relative.
func (r root) globSpelling(pattern string, matches []string) []string {
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	if strings.HasPrefix(filepath.ToSlash(pattern), "/") {
		for _, match := range matches {
			out = append(out, "/"+match)
		}
		return out
	}
	prefix := r.rt.WorkDir()
	if prefix == "" || prefix == "." {
		return matches
	}
	for _, match := range matches {
		out = append(out, strings.TrimPrefix(match, prefix+"/"))
	}
	return out
}

// resolveWrite refuses a write path outside writable_paths.
func (r root) resolveWrite(fn, p string) (string, error) {
	name := r.resolve(p)
	if r.writableAllows(name) {
		return name, nil
	}
	return name, fmt.Errorf("%s(%s): writable_paths allows %s", fn, p, strings.Join(r.writable, ", "))
}

// writableAllows reports a path inside a writable root, an empty list permitting all.
func (r root) writableAllows(name string) bool {
	if len(r.writable) == 0 {
		return true
	}
	for _, allowed := range r.writable {
		if Within(name, allowed) {
			return true
		}
	}
	return false
}

// Within reports whether name is dir or something beneath. The separator
// keeps a sibling out: /srv/site/uploads-old is not under /srv/site/upload.
func Within(name, dir string) bool {
	if name == dir {
		return true
	}
	return strings.HasPrefix(name, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// HostPath maps a path a script supplied onto the host filesystem beneath dir.
// An empty path names nothing and answers empty.
func HostPath(rt *runner.Runtime, dir, p string) string {
	if p == "" {
		return ""
	}
	return filepath.Join(dir, filepath.FromSlash(rt.ResolvePath(p)))
}

// WritableRoots resolves the configured writable_paths against the shims' root
// directory. An entry is a path inside the project, uploads naming the
// project's uploads directory.
//
// An absolute entry survives verbatim and then matches no file: resolve clamps
// the comparison into dir. No entry grants a write outside the root.
func WritableRoots(dir string, paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	roots := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if filepath.IsAbs(p) {
			roots = append(roots, filepath.Clean(p))
			continue
		}
		clean := path.Clean("/" + filepath.ToSlash(p))
		roots = append(roots, filepath.Join(dir, filepath.FromSlash(clean)))
	}
	return roots
}
