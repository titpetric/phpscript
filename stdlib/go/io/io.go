// Package io binds the readers of go's io package under the Runtime\Go\IO
// area. A script reaches them with `import "io";` and calls
// `io.ReadAll($body)` over a body `Runtime\Go\Net\HTTP` answered with.
package io

import (
	goio "io"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// Namespace is the area the import installs.
const Namespace = model.GoNamespace + `\IO`

// Set is what the package binds. A go reader is a value a binding answered
// with, never one a script built, and the write half of the package takes a
// writer no script holds.
var Set = bindings.For("io").
	// Runtime\Go\IO\ReadAll reads $reader to the end and returns the bytes as a string.
	Bind("ReadAll", goio.ReadAll).
	// Runtime\Go\IO\ReadFull reads exactly $count bytes of $reader, or throws.
	Bind("ReadFull", readFull)

func init() {
	Set.Install()
}

// readFull reads count bytes of reader, as io.ReadFull does over a buffer a
// script has no spelling for.
func readFull(reader goio.Reader, count int) ([]byte, error) {
	buf := make([]byte, count)
	n, err := goio.ReadFull(reader, buf)
	return buf[:n], err
}
