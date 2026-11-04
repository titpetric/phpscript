// Package clean registers a binding of every shape the rules allow. The go
// tool never builds a testdata tree; bindlint parses it.
package clean

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// Register installs the bindings.
func Register(rt *runner.Runtime) {
	// Every shape counts, through the collection helpers.
	rt.RegisterFunc("count_any", func(value any) int64 {
		n, _ := model.LenValues(value)
		return int64(n)
	})
	// The options bag reads by key, whatever built it.
	rt.RegisterFunc("read_options", func(value any) string {
		found := ""
		model.RangeValues(value, func(key, val any) bool {
			if fmt.Sprint(key) == "path" {
				found = fmt.Sprint(val)
			}
			return true
		})
		return found
	})
	// A mutator needs the one shape that can grow, and refuses the rest with
	// an error the script sees as a throw.
	rt.RegisterFunc("push", func(array any, value any) (int64, error) {
		a, err := arrayTarget("push", array)
		if err != nil {
			return 0, err
		}
		a.Append(value)
		return int64(a.Len()), nil
	})
	// A type switch that answers for the native shape in a case of its own.
	rt.RegisterFunc("encode", func(value any) string {
		switch v := value.(type) {
		case *model.Array:
			return fmt.Sprint(v.Len())
		case []any:
			return fmt.Sprint(len(v))
		case map[string]any:
			return fmt.Sprint(len(v))
		}
		return ""
	})
	rt.RegisterConstructor("Bag", NewBag)
}

// arrayTarget is the guard the mutators share: the narrowing is loud.
func arrayTarget(name string, array any) (*model.Array, error) {
	a, ok := array.(*model.Array)
	if !ok || a == nil {
		return nil, fmt.Errorf("%s: expected an array, got %T", name, array)
	}
	return a, nil
}

// Bag is a registered class.
type Bag struct {
	len int
}

// NewBag constructs a Bag.
func NewBag() *Bag {
	return &Bag{}
}

// Add counts any collection a script passes.
func (b *Bag) Add(value any) int64 {
	n, _ := model.LenValues(value)
	b.len += n
	return int64(b.len)
}
