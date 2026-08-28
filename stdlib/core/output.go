package core

import (
	"io"
	"math"
	"reflect"
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
	// var_export renders $value as parsable PHP source; with $return true the source is returned, otherwise it is written to the output and null is returned.
	rt.RegisterFunc("var_export", func(value any, ret ...any) (any, error) {
		w := newValueWriter()
		w.export(value, 0)
		if len(ret) > 0 && phpval.Truthy(ret[0]) {
			return w.b.String(), nil
		}
		_, err := io.WriteString(rt.Output(), w.b.String())
		return nil, err
	})
	// var_dump writes each argument to the output annotated with its type, its element count and, for a string, its length in bytes.
	rt.RegisterFunc("var_dump", func(values ...any) error {
		w := newValueWriter()
		for _, value := range values {
			w.dump(value, 0)
		}
		_, err := io.WriteString(rt.Output(), w.b.String())
		return err
	})
	// print_r renders $value for reading rather than for parsing; with $return true the text is returned, otherwise it is written to the output and true is returned.
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
	// handle, assigned when the object is created; nothing in this runtime
	// records one, so the numbering here is per call and in the order the
	// dump reaches them. Two calls therefore both start at #1 where PHP would
	// count on.
	ids map[uintptr]int
	// active holds the objects on the path from the root, so a graph that
	// refers back to itself prints *RECURSION* rather than looping forever.
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
			// object but recurses at two, so a nested array under a property
			// sits one column to the left of its own key. Odd, but it is what
			// var_export emits and what a fixture diff will hold this to.
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
// value occupies whole lines, so the caller never has to close one.
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
		// string(2), which is what tells a script its encoding went wrong.
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
		// A composite already ended its line with ")\n", so this newline is
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

// isComposite reports whether a value renders as more than one line, which is
// what decides whether var_export breaks after the arrow.
func isComposite(value any) bool {
	return model.IsCollection(value) || isObjectLike(value)
}

// isObjectLike reports whether a value prints as an object. It repeats the
// test is_object() makes rather than sharing it, so this file stands alone; a
// callable is included because PHP's Closure is an object too, and a script
// that dumps one expects to be told so.
func isObjectLike(value any) bool {
	if value == nil {
		return false
	}
	if _, ok := value.(*model.Object); ok {
		return true
	}
	if model.IsCollection(value) {
		return false
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Func {
		return true
	}
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	return rv.Kind() == reflect.Struct
}

// classOf reports the class name an object prints under: the declared name for
// an interpreted object, the Go type name for one a binding returned, and
// Closure for a callable, matching what get_class would answer.
func classOf(value any) string {
	if object, ok := value.(*model.Object); ok && object.Class != nil {
		return object.Class.Name
	}
	t := reflect.TypeOf(value)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Func {
		return "Closure"
	}
	return t.Name()
}

// objectKey returns the address identifying an object, and whether it has one.
// A struct passed by value has no identity and cannot close a cycle, so it is
// neither numbered nor guarded.
func objectKey(value any) (uintptr, bool) {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Func:
		if rv.IsNil() {
			return 0, false
		}
		return rv.Pointer(), true
	}
	return 0, false
}

// objectField is one property of an object. scope carries var_dump's
// visibility annotation (":\"Class\":private", ":protected"), empty for a
// public property.
type objectField struct {
	name  string
	scope string
	value any
}

// objectFields returns an object's properties in the order it reads them back:
// the declared fields first, in declaration order, then the ones the script
// added, in the order it added them. A dynamic property carries no scope
// annotation, because only a declaration can name a visibility.
func objectFields(value any) []objectField {
	if object, ok := value.(*model.Object); ok {
		fields := make([]objectField, 0, object.Len())
		object.Range(func(name string, val any) bool {
			scope := ""
			if object.Class != nil {
				if field, ok := object.Class.Field(name); ok {
					scope = fieldScope(object.Class.Name, field.Visibility)
				}
			}
			fields = append(fields, objectField{name, scope, val})
			return true
		})
		return fields
	}

	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	// An unexported Go field is not reachable from PHP at all, so it is not a
	// private property: it is not a property.
	rt := rv.Type()
	fields := make([]objectField, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		if !rt.Field(i).IsExported() {
			continue
		}
		fields = append(fields, objectField{rt.Field(i).Name, "", rv.Field(i).Interface()})
	}
	return fields
}

func fieldScope(class, visibility string) string {
	switch visibility {
	case "private":
		return ":\"" + class + "\":private"
	case "protected":
		return ":protected"
	}
	return ""
}

// exportKey renders an array key or property name as var_export writes it: an
// integer bare, anything else single-quoted.
func exportKey(key any) string {
	switch x := key.(type) {
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return exportString(phpval.String(key))
}

// dumpKey renders an array key as var_dump writes it: an integer bare, a
// string in double quotes.
func dumpKey(key any) string {
	switch x := key.(type) {
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return "\"" + phpval.String(key) + "\""
}

func exportScalar(value any) string {
	switch x := value.(type) {
	case nil:
		return "NULL"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return exportFloat(x)
	case string:
		return exportString(x)
	}
	return exportString(phpval.String(value))
}

// exportString single-quotes a string the way var_export does: only the quote
// and the backslash are escaped, so a newline stays a literal newline.
func exportString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// exportFloat keeps a float readable back as a float. Without the fraction,
// var_export(1.0) would emit "1", which PHP would parse as an int.
func exportFloat(f float64) string {
	s := phpFormatFloat(f, -1)
	if math.IsInf(f, 0) || math.IsNaN(f) || strings.ContainsAny(s, ".E") {
		return s
	}
	return s + ".0"
}

// printString renders a scalar the way print_r and echo do: true is 1, false
// and null are nothing, and a float carries precision=14 digits rather than
// var_dump's round-tripping ones.
func printString(value any) string {
	switch x := value.(type) {
	case nil:
		return ""
	case bool:
		if x {
			return "1"
		}
		return ""
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return phpFormatFloat(x, 14)
	}
	return phpval.String(value)
}

// phpFormatFloat renders a float the way PHP's php_gcvt does, with precision
// significant digits; a negative precision asks for the fewest digits that
// read back as the same double, which is what serialize_precision=-1 means for
// var_dump and var_export. echo and print_r pass 14 instead, so 0.1+0.2 is 0.3
// for one and 0.30000000000000004 for the other.
//
// The exponent form is chosen from where the decimal point falls, not from
// Go's 'g' thresholds: PHP switches when the point sits before the third
// leading zero or past the last significant digit, so 1e16 stays written out
// and 1e17 does not. Its mantissa always carries a fraction and its exponent
// no leading zero, giving 1.0E+100 where Go writes 1e+100.
func phpFormatFloat(f float64, precision int) string {
	switch {
	case math.IsInf(f, 1):
		return "INF"
	case math.IsInf(f, -1):
		return "-INF"
	case math.IsNaN(f):
		return "NAN"
	}

	ndigit, digits := precision, precision-1
	if precision < 0 {
		// The shortest round-tripping form never needs more than 17 digits,
		// and 17 is the width PHP compares the decimal point against.
		ndigit, digits = 17, -1
	}
	// 'e' puts the significant digits and the decimal exponent in a fixed
	// place, which is what the two decisions below need; 'g' would already
	// have made them, differently.
	s := strconv.FormatFloat(f, 'e', digits, 64)
	sign := ""
	if s[0] == '-' {
		sign, s = "-", s[1:]
	}
	mark := strings.IndexByte(s, 'e')
	exp, err := strconv.Atoi(s[mark+1:])
	if err != nil {
		return s
	}
	mantissa := strings.TrimRight(strings.Replace(s[:mark], ".", "", 1), "0")
	if mantissa == "" {
		mantissa = "0"
	}
	// point is the position of the decimal point relative to the digits:
	// 0.<digits> * 10^point, PHP's own convention for the comparison below.
	point := exp + 1

	if point < -3 || point > ndigit {
		fraction := mantissa[1:]
		if fraction == "" {
			fraction = "0"
		}
		return sign + mantissa[:1] + "." + fraction + "E" + expSign(exp) + strconv.Itoa(abs(exp))
	}
	switch {
	case point <= 0:
		return sign + "0." + strings.Repeat("0", -point) + mantissa
	case point >= len(mantissa):
		return sign + mantissa + strings.Repeat("0", point-len(mantissa))
	default:
		return sign + mantissa[:point] + "." + mantissa[point:]
	}
}

func expSign(exp int) string {
	if exp < 0 {
		return "-"
	}
	return "+"
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
