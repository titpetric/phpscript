// Package pexec exposes process execution to PHP: the ref.exec.php functions
// that run a command through the shell, the two that quote for it, and
// posix_getpid and getmypid, which name the running process.
//
// Running a command leaves the fs.FS sandbox behind on purpose. A process reads
// and writes with the permissions of the user running the server, and
// writable_paths does not reach it; a host that runs untrusted scripts leaves
// this package out. What the runtime does say is where a command starts, which
// is the working directory chdir() moved, resolved onto the host.
package pexec

import (
	"os"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the process bindings to stdlib.Register, rooted at the
// process working directory, which is where a CLI run runs anyway.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs the process bindings rooted at the process working
// directory. A host serving scripts out of a project directory binds that
// instead, with RegisterRoot.
func Register(rt *runner.Runtime) {
	RegisterRoot(rt, "")
}

// RegisterRoot installs the process bindings with commands rooted at dir, the
// directory the filesystem shims are bound to. It replaces whatever root was
// installed before it, so a host calls it after stdlib.Register, the way
// stdlib.RegisterFS does for the filesystem.
//
// An empty dir leaves a command in the process working directory, which is what
// a host that bound no directory of its own has to fall back to.
func RegisterRoot(rt *runner.Runtime, dir string) {
	r := root{rt: rt, dir: dir}

	registerExec(rt, r)
	registerEscape(rt)
	registerProcess(rt)
}

// root is the directory a command starts in, and the runtime it reads the
// working directory off. It is read per call rather than resolved once, because
// chdir moves it while the script runs.
type root struct {
	rt  *runner.Runtime
	dir string
}

// registerProcess installs what names the running process rather than a
// command it starts.
func registerProcess(rt *runner.Runtime) {
	pid := func() int64 { return int64(os.Getpid()) }

	// posix_getpid returns the process id of the running interpreter.
	rt.RegisterFunc("posix_getpid", pid)

	// getmypid returns the process id of the running interpreter, the core spelling of posix_getpid.
	rt.RegisterFunc("getmypid", pid)
}
