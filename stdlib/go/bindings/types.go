package bindings

import (
	"reflect"
)

// Type discovery is why a binding package names functions and no types. The
// signature of `http.NewRequest` names `*http.Request`, whose fields name
// `http.Header` and `url.URL`, and the types a script can reach are the ones
// the bound signatures already imply. The walk collects them; a type no
// signature mentions binds under no name.
//
// The technique is gozero's, which discovers the types behind its bindings the
// same way (github.com/titpetric/gozero, docs/types.md).

// discoverDepth bounds the type walk at six levels from a bound signature.
const discoverDepth = 6

// Types returns the types of this package the signatures reach, by go's name.
func (s *Set) Types() map[string]reflect.Type {
	w := &walk{path: s.path, out: map[string]reflect.Type{}, depth: map[reflect.Type]int{}}
	for _, kind := range s.seeds {
		w.visit(kind, 1)
	}
	for _, entry := range s.entries {
		w.visit(reflect.TypeOf(entry.Value), 1)
	}
	return w.out
}

// walk collects the types bound signatures reach into one name per type.
type walk struct {
	path  string
	out   map[string]reflect.Type
	depth map[reflect.Type]int
}

// visit records kind under its own name when the package owns it, then walks its reach.
func (w *walk) visit(kind reflect.Type, depth int) {
	if kind == nil || depth > discoverDepth {
		return
	}
	if seen, ok := w.depth[kind]; ok && seen <= depth {
		return // also what stops a recursive type looping
	}
	w.depth[kind] = depth
	if kind.Name() != "" && kind.PkgPath() == w.path {
		w.out[kind.Name()] = kind
	}
	switch kind.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		w.visit(kind.Elem(), depth+1)
	case reflect.Map:
		w.visit(kind.Key(), depth+1)
		w.visit(kind.Elem(), depth+1)
	case reflect.Struct:
		// A field type counts because a script reads a field: `$req->Header`
		// has to name the type behind it.
		for i := range kind.NumField() {
			if field := kind.Field(i); field.PkgPath == "" {
				w.visit(field.Type, depth+1)
			}
		}
	case reflect.Func:
		for i := range kind.NumIn() {
			w.visit(kind.In(i), depth+1)
		}
		for i := range kind.NumOut() {
			w.visit(kind.Out(i), depth+1)
		}
	}
	// A method's signature names types a script reaches by calling it.
	for i := range kind.NumMethod() {
		w.visit(kind.Method(i).Type, depth+1)
	}
}
