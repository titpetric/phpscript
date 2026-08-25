package runner

import (
	"reflect"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// phpInstanceOf evaluates `$value instanceof Class` as class-name equality:
// it is `get_class($value) == "Class"`, compared case-insensitively as class
// names are everywhere else.
//
// class is the name written on the right, or the value an expression there
// produced, since PHP accepts a class name held in a variable and an object to
// take the class of. A scalar, null and an array are an instance of nothing.
//
// An interface name is answered from the list the class declared, including the
// names those interfaces extend, so `$store instanceof Reader` is true when
// Store implements an interface extending Reader. That list is names, and no
// member arrives through it.
//
// Nothing follows `extends` on a class, because there is no inheritance to
// follow: a class declaring `extends RuntimeException` is an instance of itself
// and of nothing else. `instanceof Throwable` is false, Throwable being an
// interface no declaration in the program lists. See docs/design.md.
func phpInstanceOf(value, class any) bool {
	want := instanceOfName(class)
	if want == "" {
		return false
	}
	if have := instanceOfClass(value); have != "" && strings.EqualFold(want, have) {
		return true
	}
	// An interface name is not a class name, so it is answered from the list
	// the declaration recorded. That list is names, not members: a class
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

// instanceOfClass returns the class name of a value, or "" when the value is
// not an object. A scalar is never an instance of anything, which is what makes
// `1 instanceof Foo` false rather than an error.
func instanceOfClass(value any) string {
	switch v := value.(type) {
	case *model.Object:
		if v.Class != nil {
			return v.Class.Name
		}
		return ""
	case nil, bool, string, int, int64, float64, *model.Array:
		return ""
	}
	if err, ok := value.(error); ok {
		if class, isThrowable := throwableClassOf(err); isThrowable {
			return class
		}
	}
	return goTypeName(value)
}

// phpClassName returns the class name a script sees for a value, which is the
// declared name for an interpreted object and the Go type name for a
// host-backed one.
func phpClassName(value any) string {
	if class := instanceOfClass(value); class != "" {
		return class
	}
	return phpDebugType(value)
}

// goTypeName returns the declared name of the Go type behind a value, which is
// the class name a script sees for a host-backed object: `new Database("app")`
// produces a *database.Database and `$db instanceof Database` is true.
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
