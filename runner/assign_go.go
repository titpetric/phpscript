package runner

import (
	"fmt"
	"reflect"

	"github.com/titpetric/phpscript/model"
)

// This file writes into a binding's native Go value by reflection: map
// entries, slice elements and struct fields, and the unset that reslices a
// slice around the removed element. runner_assign.go writes the script-owned
// storage, *model.Array, directly.

// assignGoIndex writes into a native Go collection returned by a binding. Go
// maps accept new and replacement keys (they are reference types and the script
// observes the write), and existing slice elements are addressable through the
// shared backing array. A slice cannot grow through the interface value holding
// it and callers reject `$a[] =` before reaching here.
func assignGoIndex(base, key any, value func(current any) (any, error)) error {
	if keyed, ok := base.(model.Keyed); ok {
		name := keyString(key)
		next, err := value(keyed.Read(name))
		if err != nil {
			return err
		}
		keyed.Write(name, next)
		return nil
	}
	rv := reflect.ValueOf(base)
	switch rv.Kind() {
	case reflect.Map:
		if rv.IsNil() {
			return fmt.Errorf("assign: cannot write to a nil %T", base)
		}
		mapKey, ok := coerceArg(rv.Type().Key(), normalizeKey(key))
		if !ok || !mapKey.Type().AssignableTo(rv.Type().Key()) {
			return fmt.Errorf("assign: cannot use %T as a key of %T", key, base)
		}
		var current any
		if existing := rv.MapIndex(mapKey); existing.IsValid() {
			current = existing.Interface()
		}
		updated, err := value(current)
		if err != nil {
			return err
		}
		next, ok := coerceArg(rv.Type().Elem(), updated)
		if !ok || !next.Type().AssignableTo(rv.Type().Elem()) {
			return fmt.Errorf("assign: cannot assign to an element of %T", base)
		}
		rv.SetMapIndex(mapKey, next)
		return nil
	case reflect.Slice, reflect.Array:
		index := toInt(key)
		if index < 0 || index >= int64(rv.Len()) {
			return fmt.Errorf("assign: index %d is out of range for %T", index, base)
		}
		element := rv.Index(int(index))
		updated, err := value(element.Interface())
		if err != nil {
			return err
		}
		next, ok := coerceArg(element.Type(), updated)
		if !element.CanSet() || !ok || !next.Type().AssignableTo(element.Type()) {
			return fmt.Errorf("assign: cannot assign to an element of %T", base)
		}
		element.Set(next)
		return nil
	}
	return fmt.Errorf("assign: target is not an array")
}

// unsetGoIndex removes key from a native Go collection, returning the
// container to store back and whether it changed.
//
// A map is a reference type and the delete is in place and it answers nil.
// A slice cannot hold a hole where PHP leaves one, and cannot shrink through
// the interface value holding it either and it answers a reslice around the
// element for the caller to assign. Anything with nothing to remove, a
// scalar, a nil map, a key that is not there, answers nil and
// unset($x[$k]) runs unconditionally.
func unsetGoIndex(base, key any) (replacement any, changed bool) {
	if keyed, ok := base.(model.Keyed); ok {
		keyed.Delete(keyString(key))
		return nil, false
	}
	rv := reflect.ValueOf(base)
	switch rv.Kind() {
	case reflect.Map:
		if rv.IsNil() {
			return nil, false
		}
		mapKey, ok := coerceArg(rv.Type().Key(), normalizeKey(key))
		if !ok || !mapKey.Type().AssignableTo(rv.Type().Key()) {
			return nil, false
		}
		rv.SetMapIndex(mapKey, reflect.Value{})
		return nil, false

	case reflect.Slice:
		index, ok := normalizeKey(key).(int64)
		if !ok || index < 0 || index >= int64(rv.Len()) {
			return nil, false
		}
		// PHP keeps the keys around the hole and phpscript renumbers, as
		// array_values() does, because a slice is dense. docs/README.md records
		// the divergence.
		shorter := reflect.MakeSlice(rv.Type(), 0, rv.Len()-1)
		shorter = reflect.AppendSlice(shorter, rv.Slice(0, int(index)))
		shorter = reflect.AppendSlice(shorter, rv.Slice(int(index)+1, rv.Len()))
		return shorter.Interface(), true
	}
	return nil, false
}

func assignGoField(base any, name string, value func(any) (any, error)) error {
	rv := reflect.ValueOf(base)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return fmt.Errorf("assign: %q is not an object property", name)
	}
	object := reflect.Indirect(rv)
	if object.Kind() != reflect.Struct {
		return fmt.Errorf("assign: %q is not an object property", name)
	}
	field := fieldByNameFold(object, name)
	if !field.IsValid() || !field.CanSet() || !field.CanInterface() {
		return fmt.Errorf("assign: %q is not a writable object property", name)
	}
	raw, err := value(field.Interface())
	if err != nil {
		return err
	}
	next, ok := coerceArg(field.Type(), raw)
	if !ok || !next.Type().AssignableTo(field.Type()) {
		return fmt.Errorf("assign: cannot assign %T to property %q", raw, name)
	}
	field.Set(next)
	return nil
}
