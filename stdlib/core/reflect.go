package core

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the reflection functions to stdlib.Register.
func init() {
	runner.RegisterBinding(registerReflection)
}

func registerReflection(rt *runner.Runtime) {
	// get_class returns the class name of $object, or false when $object is missing or null.
	rt.RegisterFunc("get_class", func(object ...any) any {
		if len(object) == 0 || object[0] == nil {
			return false
		}
		return classNameOf(object[0])
	})
	// get_parent_class always returns false; phpscript does not track a parent class.
	rt.RegisterFunc("get_parent_class", func(_ ...any) any { return false })
	// get_object_vars returns the properties of $object as an array, declared fields first; a non-object yields an empty array.
	rt.RegisterFunc("get_object_vars", func(object any) *model.Array {
		out := model.NewArray()
		if obj, ok := object.(*model.Object); ok {
			for _, field := range obj.Class.Fields {
				if v, ok := obj.Props[field.Name]; ok {
					out.Set(field.Name, v)
				}
			}
			for name, v := range obj.Props {
				if _, seen := out.Get(name); !seen {
					out.Set(name, v)
				}
			}
		}
		return out
	})
	// method_exists reports whether $object_or_class, an object or a class name, has method $method.
	rt.RegisterFunc("method_exists", func(objectOrClass any, method string) bool {
		if name, ok := objectOrClass.(string); ok {
			return rt.MethodExists(name, method)
		}
		if object, ok := objectOrClass.(*model.Object); ok && object.Class != nil {
			return rt.MethodExists(object.Class.Name, method)
		}
		value := reflect.ValueOf(objectOrClass)
		if !value.IsValid() {
			return false
		}
		for i := 0; i < value.NumMethod(); i++ {
			if strings.EqualFold(value.Type().Method(i).Name, method) {
				return true
			}
		}
		return false
	})
	// property_exists reports whether object $object_or_class has property $property, set or declared; a class name is not accepted and returns false.
	rt.RegisterFunc("property_exists", func(objectOrClass any, property string) bool {
		object, ok := objectOrClass.(*model.Object)
		if !ok {
			return false
		}
		if _, ok := object.Props[property]; ok {
			return true
		}
		if object.Class == nil {
			return false
		}
		for _, field := range object.Class.Fields {
			if field.Name == property {
				return true
			}
		}
		return false
	})
	// PHP hands out an opaque, per-object identity. A pointer is exactly that,
	// and it is stable for as long as the object is alive, which is the only
	// guarantee PHP makes either.
	rt.RegisterFunc("spl_object_hash", func(value any) string {
		return fmt.Sprintf("%032x", objectIdentity(value))
	})
	// spl_object_id returns a numeric identity for $object, stable for as long as the object is alive.
	rt.RegisterFunc("spl_object_id", func(object any) int64 {
		return int64(objectIdentity(object))
	})
}

// classNameOf reports the PHP class name of a value: the declared name for an
// interpreted object, and the Go type name for a host-backed one.
func classNameOf(value any) string {
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
	return t.Name()
}

// objectIdentity returns a stable numeric identity for an object value.
func objectIdentity(value any) uintptr {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return 0
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.UnsafePointer, reflect.Func:
		return rv.Pointer()
	}
	return 0
}
