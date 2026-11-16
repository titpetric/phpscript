// Package os binds the process environment calls of Go's os package as the
// Runtime\Go\Os namespace. A script imports it with `import "os";` and calls
// `os.Getenv($name)`; the default runtime installs none of these names.
package os

import (
	goos "os"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// Namespace is the area the import installs.
const Namespace = modelutil.GoNamespace + `\Os`

// Set is what the package binds.
var Set = bindings.For("os").
	// Runtime\Go\Os\Getenv returns the value of environment variable $key, or "" for a name the process does not hold.
	Bind("Getenv", goos.Getenv).
	// Runtime\Go\Os\Stderr is the process standard error stream.
	Bind("Stderr", goos.Stderr)

func init() {
	Set.Install()
}
