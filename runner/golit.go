package runner

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// This file builds the go runtime's composite literals: the inferred []any and
// map[string]any, and the typed slice, map and nesting of them. A class name
// in an element position holds any, the value a script stores an instance in.

// helperGoLit builds a go composite literal from its evaluated entries.
func helperGoLit(mapLit bool, keyType, valType string, items ...model.ArrayItemValue) (any, error) {
	if mapLit {
		return goLitMap(keyType, valType, items)
	}
	if goLitDimsOnly(valType) {
		return goLitInfer(items)
	}
	return goLitSlice(valType, items)
}

// goLitDimsOnly reports whether valType is `[]` nesting that names no base type.
func goLitDimsOnly(valType string) bool {
	if valType == "" {
		return false
	}
	for rest := valType; rest != ""; {
		next, ok := strings.CutPrefix(rest, "[]")
		if !ok {
			return false
		}
		rest = next
	}
	return true
}

// goLitInfer builds the slice whose element type its values name.
func goLitInfer(items []model.ArrayItemValue) (any, error) {
	values := make([]any, len(items))
	elem := anyGoType
	for i, it := range items {
		value := goLitTyped(it.Val)
		values[i] = value
		if value == nil {
			continue
		}
		kind := reflect.TypeOf(value)
		if elem == anyGoType {
			elem = kind
			continue
		}
		elem = goLitUnify(elem, kind)
	}
	out := reflect.MakeSlice(reflect.SliceOf(elem), 0, len(values))
	for _, value := range values {
		out = reflect.Append(out, goLitReflect(elem, value))
	}
	return out.Interface(), nil
}

// goLitTyped converts an inferred []any into the slice its values name.
func goLitTyped(v any) any {
	list, ok := v.([]any)
	if !ok {
		return v
	}
	pairs := make([]model.ArrayItemValue, len(list))
	for i, item := range list {
		pairs[i] = model.ArrayItemValue{Val: goLitTyped(item)}
	}
	out, err := goLitInfer(pairs)
	if err != nil {
		return v
	}
	return out
}

// goLitUnify answers the type two literals share, or any when they share none.
func goLitUnify(a, b reflect.Type) reflect.Type {
	if a == b {
		return a
	}
	if a.Kind() == reflect.Slice && b.Kind() == reflect.Slice {
		return reflect.SliceOf(goLitUnify(a.Elem(), b.Elem()))
	}
	if a.Kind() == reflect.Map && b.Kind() == reflect.Map {
		return reflect.MapOf(goLitUnify(a.Key(), b.Key()), goLitUnify(a.Elem(), b.Elem()))
	}
	if goLitInt(a) && goLitInt(b) {
		return int64GoType
	}
	if goLitNumber(a) && goLitNumber(b) {
		return float64GoType
	}
	return anyGoType
}

// goLitInt reports whether t is an integer width.
func goLitInt(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// goLitNumber reports whether t is an integer or float width.
func goLitNumber(t reflect.Type) bool {
	return goLitInt(t) || t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64
}

// goLitSlice builds the slice a list literal names.
func goLitSlice(valType string, items []model.ArrayItemValue) (any, error) {
	elemName, elemType := "", anyGoType
	if strings.HasPrefix(valType, "[]") {
		elemName = strings.TrimPrefix(valType, "[]")
		if t := goLitGoType(elemName); t != nil {
			elemType = t
		}
	}
	if elemType == anyGoType {
		out := make([]any, len(items))
		for i, it := range items {
			out[i] = it.Val
		}
		return out, nil
	}
	out := reflect.MakeSlice(reflect.SliceOf(elemType), 0, len(items))
	for _, it := range items {
		v, err := goLitConvert(elemName, it.Val)
		if err != nil {
			return nil, err
		}
		out = reflect.Append(out, goLitReflect(elemType, v))
	}
	return out.Interface(), nil
}

// goLitMap builds the map a map literal names, inferring map[string]any.
func goLitMap(keyType, valType string, items []model.ArrayItemValue) (any, error) {
	keyName := keyType
	if keyName == "" {
		keyName = "string"
	}
	key := goLitGoType(keyName)
	value := goLitGoType(valType)
	if key == nil {
		key = anyGoType
	}
	if value == nil {
		value = anyGoType
	}
	out := reflect.MakeMapWithSize(reflect.MapOf(key, value), len(items))
	for _, it := range items {
		k, err := goLitConvert(keyName, it.Key)
		if err != nil {
			return nil, err
		}
		v, err := goLitConvert(valType, it.Val)
		if err != nil {
			return nil, err
		}
		out.SetMapIndex(goLitReflect(key, k), goLitReflect(value, v))
	}
	return out.Interface(), nil
}

// goLitConvert applies a builtin go type's conversion to one literal entry.
func goLitConvert(name string, v any) (any, error) {
	if name == "" || name == "any" {
		return v, nil
	}
	if strings.HasPrefix(name, "[]") {
		return goLitConvertSlice(name, v)
	}
	if keyName, valName, ok := goLitMapNames(name); ok {
		return goLitConvertMap(keyName, valName, v)
	}
	if !modelutil.GoType(name) {
		return v, nil
	}
	var reduced any
	switch name {
	case "bool":
		reduced = phpTruthy(v)
	case "string":
		reduced = phpString(v)
	case "float32", "float64":
		reduced = toFloat(v)
	default:
		reduced = toInt(v)
	}
	folded, ok := modelutil.GoFold(name, reduced)
	if !ok {
		return nil, fmt.Errorf("go literal: cannot convert %T to %s", v, name)
	}
	return folded, nil
}

// goLitConvertSlice converts a literal entry into the slice type name names.
func goLitConvertSlice(name string, v any) (any, error) {
	elemName := strings.TrimPrefix(name, "[]")
	elemType := goLitGoType(elemName)
	if elemType == nil {
		return v, nil
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return nil, fmt.Errorf("go literal: cannot convert %T to %s", v, name)
	}
	out := reflect.MakeSlice(reflect.SliceOf(elemType), 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		item, err := goLitConvert(elemName, rv.Index(i).Interface())
		if err != nil {
			return nil, err
		}
		out = reflect.Append(out, goLitReflect(elemType, item))
	}
	return out.Interface(), nil
}

// goLitConvertMap converts a literal entry into the map type the names build.
func goLitConvertMap(keyName, valName string, v any) (any, error) {
	keyType, valType := goLitGoType(keyName), goLitGoType(valName)
	if keyType == nil {
		keyType = anyGoType
	}
	if valType == nil {
		valType = anyGoType
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Map {
		return nil, fmt.Errorf("go literal: cannot convert %T to a map", v)
	}
	out := reflect.MakeMapWithSize(reflect.MapOf(keyType, valType), rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		k, err := goLitConvert(keyName, iter.Key().Interface())
		if err != nil {
			return nil, err
		}
		value, err := goLitConvert(valName, iter.Value().Interface())
		if err != nil {
			return nil, err
		}
		out.SetMapIndex(goLitReflect(keyType, k), goLitReflect(valType, value))
	}
	return out.Interface(), nil
}

// goLitMapNames splits a map[K]V spelling into its key and value types.
func goLitMapNames(name string) (keyName, valName string, ok bool) {
	rest, ok := strings.CutPrefix(name, "map[")
	if !ok {
		return "", "", false
	}
	depth := 1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 && i+1 < len(rest) {
				return rest[:i], rest[i+1:], true
			}
		}
	}
	return "", "", false
}

var (
	anyGoType     = reflect.TypeOf((*any)(nil)).Elem()
	boolGoType    = reflect.TypeOf(false)
	stringGoType  = reflect.TypeOf("")
	int64GoType   = reflect.TypeOf(int64(0))
	float64GoType = reflect.TypeOf(float64(0))
)

// goLitGoType answers the go type a literal spelling names, nil for a class.
func goLitGoType(name string) reflect.Type {
	if rest, ok := strings.CutPrefix(name, "[]"); ok {
		elem := goLitGoType(rest)
		if elem == nil {
			return nil
		}
		return reflect.SliceOf(elem)
	}
	if keyName, valName, ok := goLitMapNames(name); ok {
		key, val := goLitGoType(keyName), goLitGoType(valName)
		if key == nil || val == nil {
			return nil
		}
		return reflect.MapOf(key, val)
	}
	switch name {
	case "any":
		return anyGoType
	case "bool":
		return boolGoType
	case "string":
		return stringGoType
	case "int", "int64":
		return int64GoType
	case "float64":
		return float64GoType
	case "int8":
		return reflect.TypeOf(int8(0))
	case "int16":
		return reflect.TypeOf(int16(0))
	case "int32", "rune":
		return reflect.TypeOf(int32(0))
	case "uint", "uint64":
		return reflect.TypeOf(uint64(0))
	case "uint8", "byte":
		return reflect.TypeOf(uint8(0))
	case "uint16":
		return reflect.TypeOf(uint16(0))
	case "uint32":
		return reflect.TypeOf(uint32(0))
	case "float32":
		return reflect.TypeOf(float32(0))
	}
	return nil
}

// goLitReflect boxes v as t, a nil entry as t's zero value.
func goLitReflect(t reflect.Type, v any) reflect.Value {
	if v == nil {
		return reflect.Zero(t)
	}
	rv := reflect.ValueOf(v)
	if rv.Type().AssignableTo(t) {
		return rv
	}
	if rv.Type().ConvertibleTo(t) {
		return rv.Convert(t)
	}
	return reflect.Zero(t)
}
