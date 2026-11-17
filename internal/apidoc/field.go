package apidoc

import (
	"reflect"
)

// Field is one exported Go struct field a script reads as a PHP property.
// Signature renders it as a property declaration.
type Field struct {
	// Name is the snake_case spelling Runtime.structField resolves back to
	// the Go field, and what a script writes to reach it.
	Name    string   `json:"name"`
	GoName  string   `json:"go_name,omitempty"`
	Type    string   `json:"type,omitempty"` // PHP type name
	Comment []string `json:"comment,omitempty"`
}

// reflectFields lists the exported properties of a Go type in declaration order.
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

// fieldType returns a field's PHP type, naming a registered class directly.
func fieldType(t reflect.Type, classTypes map[reflect.Type]string) string {
	if class, ok := classTypes[t]; ok {
		return class
	}
	return phpTypeReflect(t)
}
