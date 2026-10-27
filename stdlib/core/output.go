package core

import (
	"io"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the value-printing functions to stdlib.Register.
func init() {
	runner.RegisterBinding(registerOutput)
}

func registerOutput(rt *runner.Runtime) {
	// var_export renders $value as parsable PHP source; with $return true it
	// returns the source, otherwise it writes the source to the output and
	// returns null.
	rt.RegisterFunc("var_export", func(value any, ret ...any) (any, error) {
		w := newValueWriter()
		w.export(value, 0)
		if len(ret) > 0 && phpval.Truthy(ret[0]) {
			return w.b.String(), nil
		}
		_, err := io.WriteString(rt.Output(), w.b.String())
		return nil, err
	})
	// var_dump writes each argument to the output annotated with its type,
	// its element count and, for a string, its length in bytes.
	rt.RegisterFunc("var_dump", func(values ...any) error {
		w := newValueWriter()
		for _, value := range values {
			w.dump(value, 0)
		}
		_, err := io.WriteString(rt.Output(), w.b.String())
		return err
	})
	// print_r renders $value for a reader, in a shape nothing parses. With
	// $return true it returns the text; otherwise it writes the text to the
	// output and returns true.
	rt.RegisterFunc("print_r", func(value any, ret ...any) (any, error) {
		w := newValueWriter()
		w.printR(value, 0)
		if len(ret) > 0 && phpval.Truthy(ret[0]) {
			return w.b.String(), nil
		}
		if _, err := io.WriteString(rt.Output(), w.b.String()); err != nil {
			return nil, err
		}
		return true, nil
	})
}

// valueWriter renders one call of var_export, var_dump or print_r. The three
// share a buffer type because they share the walk: the same object graph, the
// same cycle problem, the same object numbering.
type valueWriter struct {
	b *strings.Builder
	// ids numbers objects for var_dump's "#n". PHP's number is the object
	// handle it assigns at construction; this runtime records no handle,
	// and the numbering here is per call and in the order the dump reaches
	// them. Two calls both start at #1 where PHP would count on.
	ids map[uintptr]int
	// active holds the objects on the path from the root and a graph that
	// refers back to itself prints *RECURSION* and never loops forever.
	active map[uintptr]bool
}

func newValueWriter() *valueWriter {
	return &valueWriter{b: &strings.Builder{}}
}

// enter records an object as being on the current path and returns its number
// along with whether the walk may descend into it.
func (w *valueWriter) enter(value any) (id int, ok bool) {
	key, addressable := objectKey(value)
	if !addressable {
		return 0, true
	}
	if w.active[key] {
		return 0, false
	}
	if w.ids == nil {
		w.ids = map[uintptr]int{}
		w.active = map[uintptr]bool{}
	}
	id, seen := w.ids[key]
	if !seen {
		id = len(w.ids) + 1
		w.ids[key] = id
	}
	w.active[key] = true
	return id, true
}

func (w *valueWriter) leave(value any) {
	if key, addressable := objectKey(value); addressable {
		delete(w.active, key)
	}
}

// export writes value as PHP source at the given indent, backing var_export.
func (w *valueWriter) export(value any, indent int) {
	switch {
	case model.IsCollection(value):
		w.pad(indent)
		w.b.WriteString("array (\n")
		model.RangeValues(value, func(key, val any) bool {
			w.exportEntry(exportKey(key), val, indent+2, indent+2)
			return true
		})
		w.pad(indent)
		w.b.WriteByte(')')
	case isObjectLike(value):
		_, ok := w.enter(value)
		if !ok {
			w.b.WriteString("NULL")
			return
		}
		defer w.leave(value)
		w.pad(indent)
		open, shut := "\\"+classOf(value)+"::__set_state(array(\n", "))"
		if classOf(value) == "stdClass" {
			open, shut = "(object) array(\n", ")"
		}
		w.b.WriteString(open)
		for _, field := range objectFields(value) {
			// PHP indents an object's property names three spaces past the
			// object but recurses at two and a nested array under a property
			// sits one column to the left of its own key. Odd, but var_export
			// emits that and a fixture diff holds this to it.
			w.exportEntry(exportKey(field.name), field.value, indent+3, indent+2)
		}
		w.pad(indent)
		w.b.WriteString(shut)
	default:
		w.b.WriteString(exportScalar(value))
	}
}

// exportEntry writes one "key => value," line. A composite value moves to its
// own line, leaving the trailing space after the arrow that var_export emits.
func (w *valueWriter) exportEntry(key string, value any, keyIndent, valIndent int) {
	w.pad(keyIndent)
	w.b.WriteString(key)
	w.b.WriteString(" => ")
	if isComposite(value) {
		w.b.WriteByte('\n')
		w.export(value, valIndent)
	} else {
		w.b.WriteString(exportScalar(value))
	}
	w.b.WriteString(",\n")
}

// dump writes value with its type at the given indent, backing var_dump. Every
// value occupies whole lines and the caller never has to close one.
func (w *valueWriter) dump(value any, indent int) {
	w.pad(indent)
	switch x := value.(type) {
	case nil:
		w.b.WriteString("NULL\n")
		return
	case bool:
		w.b.WriteString("bool(" + strconv.FormatBool(x) + ")\n")
		return
	case int:
		w.b.WriteString("int(" + strconv.Itoa(x) + ")\n")
		return
	case int64:
		w.b.WriteString("int(" + strconv.FormatInt(x, 10) + ")\n")
		return
	case float64:
		w.b.WriteString("float(" + phpFormatFloat(x, -1) + ")\n")
		return
	case string:
		// PHP counts the bytes, not the characters: a two-byte é makes
		// string(2), and that is how a script sees a broken encoding.
		w.b.WriteString("string(" + strconv.Itoa(len(x)) + ") \"" + x + "\"\n")
		return
	}

	switch {
	case model.IsCollection(value):
		n, _ := model.LenValues(value)
		w.b.WriteString("array(" + strconv.Itoa(n) + ") {\n")
		model.RangeValues(value, func(key, val any) bool {
			w.pad(indent + 2)
			w.b.WriteString("[" + dumpKey(key) + "]=>\n")
			w.dump(val, indent+2)
			return true
		})
		w.pad(indent)
		w.b.WriteString("}\n")
	case isObjectLike(value):
		id, ok := w.enter(value)
		if !ok {
			w.b.WriteString("*RECURSION*\n")
			return
		}
		defer w.leave(value)
		fields := objectFields(value)
		w.b.WriteString("object(" + classOf(value) + ")#" + strconv.Itoa(id) +
			" (" + strconv.Itoa(len(fields)) + ") {\n")
		for _, field := range fields {
			w.pad(indent + 2)
			w.b.WriteString("[\"" + field.name + "\"" + field.scope + "]=>\n")
			w.dump(field.value, indent+2)
		}
		w.pad(indent)
		w.b.WriteString("}\n")
	default:
		w.b.WriteString("string(" + strconv.Itoa(len(phpval.String(value))) +
			") \"" + phpval.String(value) + "\"\n")
	}
}

// printR writes value for reading, backing print_r. depth counts array or
// object nesting; PHP moves eight columns per level and puts the entries four
// past the parenthesis.
func (w *valueWriter) printR(value any, depth int) {
	header := ""
	switch {
	case model.IsCollection(value):
		header = "Array\n"
	case isObjectLike(value):
		if _, ok := w.enter(value); !ok {
			w.b.WriteString(classOf(value) + " Object\n *RECURSION*")
			return
		}
		defer w.leave(value)
		header = classOf(value) + " Object\n"
	default:
		// A scalar has no lines of its own: the caller's entry closes it, and
		// print_r of a bare scalar returns just the text.
		w.b.WriteString(printString(value))
		return
	}

	w.b.WriteString(header)
	w.pad(8 * depth)
	w.b.WriteString("(\n")
	entry := func(key string, val any) {
		w.pad(8*depth + 4)
		w.b.WriteString("[" + key + "] => ")
		w.printR(val, depth+1)
		// A composite already ended its line with ")\n" and this newline is
		// the blank line PHP leaves after a nested array.
		w.b.WriteByte('\n')
	}
	if model.IsCollection(value) {
		model.RangeValues(value, func(key, val any) bool {
			entry(phpval.String(key), val)
			return true
		})
	} else {
		for _, field := range objectFields(value) {
			entry(field.name+strings.ReplaceAll(field.scope, "\"", ""), field.value)
		}
	}
	w.pad(8 * depth)
	w.b.WriteString(")\n")
}

func (w *valueWriter) pad(n int) {
	const spaces = "                                "
	for n > len(spaces) {
		w.b.WriteString(spaces)
		n -= len(spaces)
	}
	w.b.WriteString(spaces[:n])
}
