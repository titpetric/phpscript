package runner

import (
	"reflect"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// phpInstanceOf evaluates instanceof as case-insensitive class-name equality against declared interfaces.
func phpInstanceOf(value, class any) bool {
	want := instanceOfName(class)
	if want == "" {
		return false
	}
	if have := instanceOfClass(value); have != "" && strings.EqualFold(want, have) {
		return true
	}
	// An interface name is not a class name and the match runs against the
	// list the declaration recorded. That list is names, not members: a class
	// implementing an interface still declares everything it has.
	if obj, ok := value.(*model.Object); ok && obj.Class != nil {
		for _, name := range obj.Class.Implements {
			if strings.EqualFold(want, name) {
				return true
			}
		}
	}
	return false
}

// instanceOfName resolves the right operand to a class name.
func instanceOfName(class any) string {
	switch v := class.(type) {
	case string:
		return strings.TrimPrefix(v, "\\")
	case nil:
		return ""
	default:
		return instanceOfClass(class)
	}
}

// instanceOfClass returns the class name of a value, "" for a scalar or an array.
func instanceOfClass(value any) string {
	switch v := value.(type) {
	case *model.Object:
		if v.Class != nil {
			return v.Class.Name
		}
		return ""
	case nil, bool, string, int, int64, float64, *model.Array:
		return ""
	case *Callable:
		// A callable value is php's Closure whichever of the three it holds, and
		// get_class() says the same.
		return "Closure"
	}
	if err, ok := value.(error); ok {
		if class, isThrowable := throwableClassOf(err); isThrowable {
			return class
		}
	}
	return goTypeName(value)
}

// phpClassName returns the class name a script sees, falling back to the value's type name.
func phpClassName(value any) string {
	if class := instanceOfClass(value); class != "" {
		return class
	}
	return phpDebugType(value)
}

// goTypeName returns the declared Go type name behind a value, a pointer dereferenced.
func goTypeName(value any) string {
	t := reflect.TypeOf(value)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return ""
	}
	return t.Name()
}
