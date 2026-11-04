package apidoc

import (
	"reflect"
	"strings"
)

// reflectBinds cites the Go type behind a registered class.
//
// A bound type the standard library declares cites itself: HTTP\Request, Time
// and Regexp\Compile all do. A bound type this module declares cites the
// standard library type it wraps, and cites itself when it wraps none.
func reflectBinds(t reflect.Type) Citation {
	own := cite(t)
	if own.Name == "" || own.URL != "" {
		return own
	}
	if inner, ok := wrappedStdlib(t, own.Name); ok {
		return cite(inner)
	}
	return own
}

// wrappedStdlib finds the standard library type a struct wraps: the first
// field of a named standard library type whose name relates to the wrapper's,
// either name ending with the other. An embedded field matches on the
// embedding alone.
//
// The name test separates the wrapped value from the machinery beside it.
// core.JSONEncoder holds an io.Writer and an encoding/json.Encoder, and
// http.Server holds a sync.Mutex, a net/http.Handler and a net/http.Server.
func wrappedStdlib(t reflect.Type, owner string) (reflect.Type, bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, false
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		inner := field.Type
		for inner.Kind() == reflect.Pointer {
			inner = inner.Elem()
		}
		if inner.Name() == "" || !isStdlib(inner.PkgPath()) {
			continue
		}
		if field.Anonymous || relatedNames(owner, inner.Name()) {
			return inner, true
		}
	}
	return nil, false
}

// relatedNames reports whether one type name ends with the other, the shape a
// wrapper shares with the type it wraps: Client wraps Client, Mux wraps
// ServeMux, JSONEncoder wraps Encoder.
func relatedNames(owner, inner string) bool {
	owner, inner = strings.ToLower(owner), strings.ToLower(inner)
	return strings.HasSuffix(owner, inner) || strings.HasSuffix(inner, owner)
}

// goTypeKey names the Go type a constructor returns, the key the doc index
// holds its godoc under. An unnamed type has no key.
func goTypeKey(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() == "" {
		return ""
	}
	if t.PkgPath() == "" {
		return t.Name()
	}
	return t.PkgPath() + "." + t.Name()
}
