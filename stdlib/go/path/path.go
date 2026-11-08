// Package path binds the lexical functions of Go's path package as the
// Runtime\Go\Path namespace. They work on slash-separated names and touch no
// file system. A script imports them with `use Runtime\Go\Path;`.
package path

import (
	gopath "path"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
)

// Namespace is the name a script imports.
const Namespace = modelutil.GoNamespace + `\Path`

func init() {
	runner.RegisterNamespace(Namespace, Register)
}

func name(fn string) string { return Namespace + `\` + fn }

// Register installs the Runtime\Go\Path functions on a runtime that imported them.
func Register(rt *runner.Runtime) {
	// Runtime\Go\Path\Base returns the last element of $p.
	rt.RegisterFunc(name("Base"), func(p string) string { return gopath.Base(p) })
	// Runtime\Go\Path\Dir returns all but the last element of $p.
	rt.RegisterFunc(name("Dir"), func(p string) string { return gopath.Dir(p) })
	// Runtime\Go\Path\Ext returns the file name extension of $p, with its dot.
	rt.RegisterFunc(name("Ext"), func(p string) string { return gopath.Ext(p) })
	// Runtime\Go\Path\Clean returns the shortest path name equivalent to $p.
	rt.RegisterFunc(name("Clean"), func(p string) string { return gopath.Clean(p) })
	// Runtime\Go\Path\IsAbs reports whether $p is absolute.
	rt.RegisterFunc(name("IsAbs"), func(p string) bool { return gopath.IsAbs(p) })
	// Runtime\Go\Path\Join joins the elements into one cleaned path.
	rt.RegisterFunc(name("Join"), func(elem ...string) string { return gopath.Join(elem...) })
	// Runtime\Go\Path\Match reports whether $name matches the shell pattern.
	rt.RegisterFunc(name("Match"), func(pattern, name string) (bool, error) { return gopath.Match(pattern, name) })
}
