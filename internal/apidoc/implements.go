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

// interfaceSet is the set reflectImplements tests a bound type against. It
// holds two kinds of entry.
//
// runner.Throwable is the one contract of this module a script names, in a
// catch clause and in instanceof, and the PHP spelling goes in the clause. The
// rest are the standard library conventions a caller acts on: a value that
// prints, reads, writes, closes, marshals or answers a request. Every other
// interface of this module is a seam between two packages no script names, and
// the empty interface holds every value.
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

// reflectImplements cites every interface of interfaceSet the bound type
// satisfies, holding to the ones no other match covers: runner.Throwable
// embeds error, and a throwable cites Throwable alone.
//
// The type is the one a constructor returns, which is the value a script
// holds, and a method set reached through a pointer counts only when the
// constructor returns that pointer.
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
