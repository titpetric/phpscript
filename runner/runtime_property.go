package runner

import (
	"reflect"

	"github.com/titpetric/phpscript/model"
)

// PropertyExists reports whether a value holds the property a script names:
// an interpreted object's own properties and the declarations that outlive an
// unset, and the exported Go fields of a value a binding returned, resolved
// through structField the way a property read resolves them.
func (rt *Runtime) PropertyExists(value any, name string) bool {
	if object, ok := value.(*model.Object); ok {
		if _, set := object.Prop(name); set {
			return true
		}
		if object.Class == nil {
			return false
		}
		_, declared := object.Class.Field(name)
		return declared
	}

	t := reflect.TypeOf(value)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return false
	}
	_, found := rt.structField(t, name)
	return found
}
