// Package io binds the readers of go's io package as the Runtime\Go\IO area. A
// script imports `io`; a call such as `io.ReadAll($body)` reads a body
// `Runtime\Go\Net\HTTP` answered.
package io

import (
	goio "io"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// Namespace is the area the import installs.
const Namespace = modelutil.GoNamespace + `\IO`

// Set holds the package bindings. A go reader comes from a binding. A script
// cannot build such a value. The write half takes a writer no script holds.
var Set = bindings.For("io").
	// Runtime\Go\IO\ReadAll reads $reader to the end and returns the bytes as a string.
	Bind("ReadAll", goio.ReadAll).
	// Runtime\Go\IO\ReadFull reads exactly $count bytes of $reader, or throws.
	Bind("ReadFull", readFull)

func init() {
	Set.Install()
}

// readFull reads count bytes, as io.ReadFull does on an unnamed buffer.
func readFull(reader goio.Reader, count int) ([]byte, error) {
	buf := make([]byte, count)
	n, err := goio.ReadFull(reader, buf)
	return buf[:n], err
}
