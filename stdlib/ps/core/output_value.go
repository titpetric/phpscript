package core

import (
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
)

// isComposite reports whether value renders on more than one line.
func isComposite(value any) bool {
	return modelutil.IsCollection(value) || isObjectLike(value)
}

// isObjectLike reports whether value prints as an object, counting a callable.
func isObjectLike(value any) bool {
	if value == nil {
		return false
	}
	if _, ok := value.(*model.Object); ok {
		return true
	}
	if modelutil.IsCollection(value) {
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

// classOf returns the name an object prints under, Closure for a callable.
func classOf(value any) string {
	if object, ok := value.(*model.Object); ok && object.Class != nil {
		return object.Class.Name
	}
	// A callable value prints as php's Closure: `$this->fn(...)`, the
	// first-class callable syntax the shorter `$this->fn` abbreviates, answers
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

// objectField is one property, with var_dump's scope annotation when it has one.
type objectField struct {
	name  string
	scope string
	value any
}

// objectFields returns properties in read-back order: declared fields, then dynamic ones.
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

// exportKey renders a key as var_export does: an integer bare, a string quoted.
func exportKey(key any) string {
	switch x := key.(type) {
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return exportString(phpval.String(key))
}

// dumpKey renders a key as var_dump does: an integer bare, a string double-quoted.
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
	// A typed go value exports as the conversion that builds it, which is php
	// a file importing Runtime\Go can run: `int8(44)`.
	if width, ok := model.GoWidthOf(value); ok {
		return width.Name + "(" + phpval.String(value) + ")"
	}
	return exportString(phpval.String(value))
}

// exportString single-quotes s, escaping only the quote and the backslash.
func exportString(s string) string {
	return "'" + exportReplacer.Replace(s) + "'"
}

// exportReplacer holds one Replacer for the package across every call.
var exportReplacer = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

// exportFloat keeps a float readable as a float, adding .0 where needed.
func exportFloat(f float64) string {
	s := phpFormatFloat(f, -1)
	if math.IsInf(f, 0) || math.IsNaN(f) || strings.ContainsAny(s, ".E") {
		return s
	}
	return s + ".0"
}

// printString renders a scalar the way print_r and echo do, at precision 14.
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
