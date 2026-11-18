package apidoc

import (
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"reflect"
	"sort"

	"github.com/titpetric/phpscript/runner"
)

// interfaceSet lists every interface reflectImplements cites, with the PHP name a script types.
var interfaceSet = []struct {
	typ reflect.Type
	php string
}{
	{reflect.TypeOf((*runner.Throwable)(nil)).Elem(), "Throwable"},
	{reflect.TypeOf((*error)(nil)).Elem(), ""},
	{reflect.TypeOf((*fmt.Stringer)(nil)).Elem(), ""},
	{reflect.TypeOf((*io.Reader)(nil)).Elem(), ""},
	{reflect.TypeOf((*io.Writer)(nil)).Elem(), ""},
	{reflect.TypeOf((*io.Closer)(nil)).Elem(), ""},
	{reflect.TypeOf((*json.Marshaler)(nil)).Elem(), ""},
	{reflect.TypeOf((*json.Unmarshaler)(nil)).Elem(), ""},
	{reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem(), ""},
	{reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem(), ""},
	{reflect.TypeOf((*nethttp.Handler)(nil)).Elem(), ""},
	{reflect.TypeOf((*nethttp.RoundTripper)(nil)).Elem(), ""},
}

// reflectImplements cites each interfaceSet entry t satisfies, dropping those another match implies.
func reflectImplements(t reflect.Type) []Citation {
	matched := []reflect.Type{}
	for _, entry := range interfaceSet {
		if t.Implements(entry.typ) {
			matched = append(matched, entry.typ)
		}
	}

	out := []Citation{}
	for _, entry := range interfaceSet {
		if !t.Implements(entry.typ) || covered(entry.typ, matched) {
			continue
		}
		c := cite(entry.typ)
		c.PHP = entry.php
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Clause() < out[j].Clause() })
	return out
}

// covered reports whether another matched interface already implies this one.
func covered(want reflect.Type, matched []reflect.Type) bool {
	for _, other := range matched {
		if other != want && other.Implements(want) {
			return true
		}
	}
	return false
}
