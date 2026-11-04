package apidoc

import (
	"reflect"
)

// Field is one exported Go struct field a script reads as a PHP property.
// Signature renders it as a property declaration.
type Field struct {
	// Name is the snake_case spelling Runtime.structField resolves back to
	// the Go field, and what a script writes to reach it.
	Name    string
	GoName  string
	Type    string // PHP type name
	Comment []string
}

// reflectFields lists the properties a script reads off the Go type a
// constructor returns, in declaration order, the order `go doc` prints a
// struct in.
//
// Exported fields and no others. Runtime.structField resolves a property read
// to an exported field and reports an unexported one as absent, and a listing
// that printed one would name a property no script can read.
func reflectFields(t reflect.Type, classTypes map[reflect.Type]string) []Field {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	fields := []Field{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		fields = append(fields, Field{
			Name:   camelToSnake(f.Name),
			GoName: f.Name,
			Type:   fieldType(f.Type, classTypes),
		})
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// fieldType is the PHP type a property read answers with. A field holding a
// registered class reports that class, the way a return value does.
func fieldType(t reflect.Type, classTypes map[reflect.Type]string) string {
	if class, ok := classTypes[t]; ok {
		return class
	}
	return phpTypeReflect(t)
}
