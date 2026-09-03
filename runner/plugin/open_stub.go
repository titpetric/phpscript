//go:build !cgo

package plugin

import (
	"fmt"
)

// symbols is the part of an opened plugin the loader uses. Nothing satisfies it
// in this build.
type symbols interface {
	Lookup(name string) (any, error)
}

// openPlugin reports that this binary cannot load plugins. Go plugins need cgo
// and a dynamically linked host, and the released phpscript binary is built
// with CGO_ENABLED=0 so that it stays a single static file. Callers match on
// ErrUnsupported and skip the work rather than fail it.
func openPlugin(path string) (symbols, error) {
	return nil, fmt.Errorf("%w: built without cgo, rebuild with CGO_ENABLED=1 to load %s", ErrUnsupported, path)
}
