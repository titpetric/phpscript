package core

import (
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// isComposite reports whether a value renders as more than one line, which is
// what decides whether var_export breaks after the arrow.
func isComposite(value any) bool {
	return model.IsCollection(value) || isObjectLike(value)
}

// isObjectLike reports whether a value prints as an object. It repeats the
// test is_object() makes and shares nothing with it and this file stands
// alone; it counts a callable because PHP's Closure is an object too, and a
// script that dumps one reads it as one.
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
	// A callable value prints as php's Closure: `$this->fn(...)`, the
	// first-class callable syntax the shorter `$this->fn` stands in for, answers
	// a Closure and has no other name.
	if _, ok := value.(*runner.Callable); ok {
		return "Closure"
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
// A struct passed by value has no identity and cannot close a cycle and
// objectKey neither numbers nor guards it.
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
	// An unexported Go field never reaches PHP and it counts as no property at
	// all, let alone a private one.
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

// exportString single-quotes a string the way var_export does: it escapes only
// the quote and the backslash and a newline stays a literal newline.
func exportString(s string) string {
	return "'" + exportReplacer.Replace(s) + "'"
}

// exportReplacer holds one Replacer for the whole package. strings.NewReplacer
// compiles its table on first use behind a sync.Once and one per call means
// compiling it per call.
var exportReplacer = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

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
// and null are nothing, and a float carries precision=14 digits and not
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
