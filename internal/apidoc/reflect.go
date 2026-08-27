package apidoc

import (
	"context"
	"reflect"
	"strconv"
)

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

// reflectParams derives PHP parameters from a Go function type. Reflection
// carries no parameter names, so they are made up from the PHP type, with a
// numeric suffix when a type repeats.
func reflectParams(t reflect.Type) []Param {
	params := []Param{}
	for i := 0; i < t.NumIn(); i++ {
		in := t.In(i)
		if in == contextType {
			continue
		}
		p := Param{}
		if t.IsVariadic() && i == t.NumIn()-1 {
			p.Variadic = true
			in = in.Elem()
		}
		if in.Kind() == reflect.Func {
			if isRefSetterType(in) {
				p.ByRef = true
				params = append(params, p)
				continue
			}
			p.Type = "callable"
		} else {
			p.Type = phpTypeReflect(in)
		}
		params = append(params, p)
	}
	return nameParams(params)
}

// nameParams fills empty parameter names from the type.
func nameParams(params []Param) []Param {
	for i := range params {
		if params[i].Name == "" {
			params[i].Name = typeParamName(params[i])
		}
	}
	return dedupeParams(params)
}

// dedupeParams numbers repeated parameter names, so two ignored arguments
// render as $unused1 and $unused2 rather than twice as $unused.
func dedupeParams(params []Param) []Param {
	counts := map[string]int{}
	for _, p := range params {
		counts[p.Name]++
	}
	seen := map[string]int{}
	for i := range params {
		base := params[i].Name
		if counts[base] > 1 {
			seen[base]++
			params[i].Name = base + strconv.Itoa(seen[base])
		}
	}
	return params
}

// typeParamName is the placeholder name for an unnamed parameter.
func typeParamName(p Param) string {
	if p.ByRef {
		return "ref"
	}
	if p.Variadic {
		return "args"
	}
	switch p.Type {
	case "string":
		return "string"
	case "int", "float":
		return "num"
	case "bool":
		return "flag"
	case "array":
		return "array"
	case "callable":
		return "callback"
	case "resource":
		return "stream"
	}
	return "value"
}

// reflectReturn folds a Go function's results into one PHP type. Registered
// concrete types use their PHP class name; a trailing error is thrown, not
// returned.
func reflectReturn(t reflect.Type, classTypes map[reflect.Type]string) string {
	kept := []string{}
	for i := 0; i < t.NumOut(); i++ {
		out := t.Out(i)
		if out == errorType {
			continue
		}
		if class, ok := classTypes[out]; ok {
			kept = append(kept, class)
			continue
		}
		kept = append(kept, phpTypeReflect(out))
	}
	return returnType(kept)
}

func hasRegisteredReturn(t reflect.Type, classTypes map[reflect.Type]string) bool {
	for i := 0; i < t.NumOut(); i++ {
		if _, ok := classTypes[t.Out(i)]; ok {
			return true
		}
	}
	return false
}

// isRefSetterType matches func(any), the runner's by-reference setter.
func isRefSetterType(t reflect.Type) bool {
	return t.NumIn() == 1 && t.NumOut() == 0 && !t.IsVariadic() &&
		t.In(0).Kind() == reflect.Interface && t.In(0).NumMethod() == 0
}

// phpTypeReflect maps a Go type to the PHP type name a script sees.
func phpTypeReflect(t reflect.Type) string {
	if t.Name() == "Array" || t.Name() == "Value" {
		if t.PkgPath() == "github.com/titpetric/phpscript/model" {
			if t.Name() == "Array" {
				return "array"
			}
			return "mixed"
		}
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Func:
		return "callable"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "string"
		}
		return "array"
	case reflect.Array, reflect.Map:
		return "array"
	case reflect.Pointer:
		return phpTypeReflect(t.Elem())
	case reflect.Struct:
		return "object"
	case reflect.Interface:
		if t.NumMethod() == 0 {
			return "mixed"
		}
		return "object"
	}
	return "mixed"
}
