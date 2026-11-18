// Package os binds the process environment calls of Go's os package as the
// Runtime\Go\Os namespace. A script imports it as `import "os";`, then calls
// `os.Getenv($name)`; the default runtime installs none of these names.
package os

import (
	goos "os"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// Namespace is the area the import installs.
const Namespace = modelutil.GoNamespace + `\Os`

// Set holds the package bindings.
var Set = bindings.For("os").
	// Runtime\Go\Os\Getenv returns the value of environment variable $key, or "" for a name the process does not hold.
	Bind("Getenv", goos.Getenv).
	// Runtime\Go\Os\Stderr is the process standard error stream.
	Bind("Stderr", goos.Stderr).
	// Runtime\Go\Os\Stdout is the process standard output stream.
	Bind("Stdout", goos.Stdout).
	// Runtime\Go\Os\Stdin is the process standard input stream.
	Bind("Stdin", goos.Stdin)

func init() {
	Set.Install()
}
