package apidoc

import (
	"reflect"
	"strings"
)

// reflectBinds cites a bound type, preferring the standard library type it wraps.
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

// wrappedStdlib finds the standard library type a struct wraps, matching on name or embedding.
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

// relatedNames reports whether one lowercased type name ends with the other.
func relatedNames(owner, inner string) bool {
	owner, inner = strings.ToLower(owner), strings.ToLower(inner)
	return strings.HasSuffix(owner, inner) || strings.HasSuffix(inner, owner)
}

// goTypeKey returns the doc index key for a Go type, empty when unnamed.
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
