// Package files provides the filesystem shims phpscript exposes to PHP: path
// helpers, reads, writes, streams, and the uploaded-file functions that go with
// $_FILES. They are grouped here rather than in stdlib because they are the one
// part of the standard library bound to a directory on the host, and because a
// host that runs untrusted scripts may want to leave them out.
//
// Go's fs.FS is read-only, so writes (fopen/fwrite/mkdir) use the os package
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

// init contributes the filesystem bindings to stdlib.Register, rooted at the
// process working directory, which is where a CLI run reads and writes anyway.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs the filesystem shims rooted at the process working
// directory. A host serving scripts out of a project directory binds that
// instead, with RegisterRoot.
func Register(rt *runner.Runtime) {
	RegisterRoot(rt, ".")
}

// RegisterRoot installs the filesystem shims rooted at dir. A relative path
// from PHP resolves against dir and cannot climb out of it; an absolute path is
// taken as the script wrote it and is not confined.
//
// Writes are held to the runtime's writable_paths when it configures any, which
// is what confines an absolute path: naming one does not make it writable.
//
// It replaces whatever root was installed before it, so a host calls it after
// stdlib.Register (stdlib.RegisterFS does exactly that).
func RegisterRoot(rt *runner.Runtime, dir string) {
	r := root{rt: rt, dir: dir}
	r.writable = WritableRoots(dir, rt.WritablePaths())

	registerPaths(rt)
	registerReads(rt, r)
	registerWrites(rt, r)
	registerStreams(rt, r)
	registerCSV(rt)
	registerUploads(rt, r)
}

// root is the directory the shims are bound to, and the runtime they resolve
// reads through. Every binding that takes a path from PHP goes through it, so
// the mapping from script path to host path is stated once.
type root struct {
	rt  *runner.Runtime
	dir string

	// writable is the resolved writable_paths allowlist. An empty list means
	// no restriction, which is what a configuration that names none asks for.
	writable []string
}

// resolve maps a path a script supplied onto the host filesystem. A relative
// path is cleaned against the root, so it cannot climb out of it; an absolute
// path is the script's own business and is only cleaned.
func (r root) resolve(p string) string {
	if filepath.IsAbs(p) {
		return path.Clean(p)
	}
	clean := path.Clean("/" + filepath.ToSlash(p))
	return filepath.Join(r.dir, filepath.FromSlash(clean))
}

// fsPath maps a path a script supplied onto the source filesystem, which every
// host roots at the same directory the shims are bound to. It names the same
// file resolve does, in the other spelling, and the two are not
// interchangeable: resolve answers with a host path joined onto r.dir, while an
// fs.FS wants a slash path relative to its own root and rejects anything else,
// r.dir included. Passing resolve's answer to an fs.FS is how a read silently
// stopped going through it whenever r.dir was not ".".
//
// A relative path is cleaned against the root, so "a/../../etc/passwd" becomes
// "etc/passwd" inside it rather than escaping, which is the rule resolve
// states. The second return is false for an absolute path: resolve hands that
// one to the host untouched, and there is no way to say the same thing to an
// fs.FS, so the caller decides what a path outside the root means.
func (r root) fsPath(p string) (string, bool) {
	if p == "" || filepath.IsAbs(p) {
		return "", false
	}
	slash := filepath.ToSlash(p)
	if strings.HasPrefix(slash, "/") {
		return "", false
	}
	clean := strings.TrimPrefix(path.Clean("/"+slash), "/")
	if clean == "" {
		clean = "."
	}
	return clean, true
}

// unresolve undoes resolve over a list of host paths, so a listing answers in
// the spelling the script asked in. PHP's glob echoes the pattern's own shape
// back: a relative pattern yields relative paths. A match that is not under the
// root came from an absolute pattern and is left as it is.
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

// resolveWrite is resolve for a path a script is about to modify. A path
// outside writable_paths is an error rather than a false return: a refused
// write is a mistake in the script or an attempt to escape its allowance, and
// both are worth stopping at rather than letting the script carry on believing
// the write happened. The runtime promotes the error to a catchable exception,
// so a script that expects to be refused can try/catch it.
//
// Failures the operating system reports keep returning PHP's false, as PHP
// does.
func (r root) resolveWrite(fn, p string) (string, error) {
	name := r.resolve(p)
	if r.writableAllows(name) {
		return name, nil
	}
	return name, fmt.Errorf("%s(%s): writable_paths allows %s", fn, p, strings.Join(r.writable, ", "))
}

// writableAllows reports whether name is under one of the writable roots. An
// unconfigured allowlist permits everything, so a project that names no
// writable_paths keeps writing wherever its user may.
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

// Within reports whether name is dir or something beneath it. The separator is
// what keeps a sibling out: /srv/site/uploads-old is not under /srv/site/upload
// even though the string starts the same way.
//
// It is exported because a host has the same question to answer: a server that
// serves a writable directory has to know not to execute the PHP a script may
// have written into it.
func Within(name, dir string) bool {
	if name == dir {
		return true
	}
	return strings.HasPrefix(name, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// WritableRoots resolves the configured writable_paths against the directory
// the shims are bound to. An entry is a path inside the project, so uploads is
// the project's uploads directory and public/uploads is the one below the
// document root; an absolute entry is taken as given, for a host that writes
// somewhere outside its own tree on purpose.
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
