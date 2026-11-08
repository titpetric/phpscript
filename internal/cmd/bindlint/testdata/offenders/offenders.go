// Package offenders registers one binding of every shape the rules refuse.
// The go tool never builds a testdata tree; bindlint parses it.
package offenders

import (
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// Register installs the bindings.
func Register(rt *runner.Runtime) {
	// A parameter declared as the runtime type: the call refuses the []string
	// another binding returns.
	rt.RegisterFunc("count_array", func(array *model.Array) int64 {
		return int64(array.Len())
	})
	// A slice of them is the same finding.
	rt.RegisterFunc("count_rows", func(rows []*model.Array) int64 {
		return int64(len(rows))
	})
	// The other half: a Go-native collection refuses the array literal.
	rt.RegisterFunc("count_names", func(names []string) int64 {
		return int64(len(names))
	})
	// A narrowing the binding only reads.
	rt.RegisterFunc("read_options", func(value any) string {
		options, ok := value.(*model.Array)
		if !ok {
			return ""
		}
		if path, found := options.Get("path"); found {
			return path.(string)
		}
		return ""
	})
	// The same narrowing, one call away.
	rt.RegisterFunc("read_options_helper", func(value any) string {
		return readPath(value)
	})
	// A registration whose name a table holds names no function to read.
	for name, fn := range table() {
		rt.RegisterFunc(name, fn)
	}
	rt.RegisterConstructor("Box", NewBox)
}

// readPath is the helper the binding passes its argument to.
func readPath(value any) string {
	options, ok := value.(*model.Array)
	if !ok {
		return ""
	}
	path, _ := options.Get("path")
	return path.(string)
}

// table holds a registration bindlint cannot resolve.
func table() map[string]any {
	return map[string]any{"noop": func() {}}
}

// Box is a registered class.
type Box struct {
	items *model.Array
}

// NewBox constructs a Box.
func NewBox() *Box {
	return &Box{items: model.NewArray()}
}

// Fill is a method a script calls, with the runtime type in its signature.
func (b *Box) Fill(items *model.Array) {
	b.items = items
}

// Size reads the box and is no finding: it narrows nothing.
func (b *Box) Size() int64 {
	return int64(b.items.Len())
}
