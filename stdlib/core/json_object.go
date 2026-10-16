package core

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// jsonObject is a JSON object that keeps the order its entries arrive in.
//
// encoding/json sorts the keys of a Go map and PHP does not: json_encode writes
// an array in build order, so a row encoded for a client reads back
// in the order the script assembled it. A map cannot carry that order, which is
// why this type exists in place of a map[string]any.
type jsonObject struct {
	pairs []jsonPair
}

type jsonPair struct {
	key   string
	value any
}

func newJSONObject(size int) *jsonObject {
	return &jsonObject{pairs: make([]jsonPair, 0, size)}
}

func (o *jsonObject) add(key string, value any) {
	o.pairs = append(o.pairs, jsonPair{key: key, value: value})
}

// MarshalJSON writes the entries in insertion order, into one buffer grown to
// an estimate of the document. A key or a scalar that needs no escaping is
// appended as itself; everything else defers to json.Marshal, so the bytes
// are the ones encoding/json writes either way.
func (o *jsonObject) MarshalJSON() ([]byte, error) {
	size := 2
	for _, pair := range o.pairs {
		size += len(pair.key) + 20
	}
	buf := make([]byte, 0, size)
	buf = append(buf, '{')
	for i, pair := range o.pairs {
		if i > 0 {
			buf = append(buf, ',')
		}
		var err error
		if buf, err = appendJSONString(buf, pair.key); err != nil {
			return nil, err
		}
		buf = append(buf, ':')
		if buf, err = appendJSONValue(buf, pair.value); err != nil {
			return nil, err
		}
	}
	return append(buf, '}'), nil
}

// appendJSONValue appends one value's encoding: the scalar and collection
// shapes a handler's response is made of directly, and json.Marshal for the
// rest, floats included, whose spelling is encoding/json's own.
//
// The collections append in place, so a document reaches the stream without the
// jsonEncodeValue tree being built first and walked by reflection after. The
// bytes are the same ones either way: the scalar cases are the shapes
// encoding/json writes verbatim, everything else defers to it.
func appendJSONValue(buf []byte, v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return append(buf, "null"...), nil
	case bool:
		if x {
			return append(buf, "true"...), nil
		}
		return append(buf, "false"...), nil
	case string:
		return appendJSONString(buf, x)
	case int64:
		return strconv.AppendInt(buf, x, 10), nil
	case int:
		return strconv.AppendInt(buf, int64(x), 10), nil
	case *model.Array:
		if x == nil {
			return append(buf, "null"...), nil
		}
		if arrayIsList(x) {
			return appendJSONList(buf, x.Len(), x.Range)
		}
		return appendJSONPairs(buf, x.Len(), x.Range)
	case *model.Object:
		if x == nil {
			return append(buf, "null"...), nil
		}
		return appendJSONPairs(buf, x.Len(), func(visit func(any, any) bool) {
			x.Range(func(name string, value any) bool { return visit(name, value) })
		})
	case model.Collection:
		// A view over request data, keyed by name, so an object. Reading it is
		// what decodes the request behind it.
		if x == nil {
			return append(buf, "null"...), nil
		}
		return appendJSONPairs(buf, x.Len(), x.Range)
	case []any:
		buf = append(buf, '[')
		for i, item := range x {
			if i > 0 {
				buf = append(buf, ',')
			}
			var err error
			if buf, err = appendJSONValue(buf, item); err != nil {
				return nil, err
			}
		}
		return append(buf, ']'), nil
	}
	encoded, err := json.Marshal(jsonEncodeValue(v))
	if err != nil {
		return nil, err
	}
	return append(buf, encoded...), nil
}

// appendJSONList appends a PHP list as a JSON array. It takes the range and no
// collection, as appendJSONPairs does, so the two read the same and one walk
// serves whatever shape carries a list.
func appendJSONList(buf []byte, size int, over func(func(any, any) bool)) ([]byte, error) {
	buf = slices.Grow(buf, size*8+2)
	buf = append(buf, '[')
	first := true
	var failure error
	over(func(_, value any) bool {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf, failure = appendJSONValue(buf, value)
		return failure == nil
	})
	if failure != nil {
		return nil, failure
	}
	return append(buf, ']'), nil
}

// appendJSONPairs appends a keyed collection as a JSON object, in the order
// range visits it, which is the order the script built it.
//
// size is the entry count, for the estimate the buffer grows to ahead of the
// walk: a document appended into one allocation is the point of appending.
func appendJSONPairs(buf []byte, size int, over func(func(any, any) bool)) ([]byte, error) {
	buf = slices.Grow(buf, size*24+2)
	buf = append(buf, '{')
	first := true
	var failure error
	over(func(key, value any) bool {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		if buf, failure = appendJSONString(buf, phpval.String(key)); failure != nil {
			return false
		}
		buf = append(buf, ':')
		buf, failure = appendJSONValue(buf, value)
		return failure == nil
	})
	if failure != nil {
		return nil, failure
	}
	return append(buf, '}'), nil
}

// appendJSONString appends a string's encoding. It takes the string itself and
// no any, because a key reaching an any parameter is boxed, and a string does
// not fit an interface word: that is one allocation per key of every object a
// response is made of.
func appendJSONString(buf []byte, s string) ([]byte, error) {
	if jsonSafe(s) {
		buf = append(buf, '"')
		buf = append(buf, s...)
		return append(buf, '"'), nil
	}
	encoded, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return append(buf, encoded...), nil
}

// jsonSafe reports a string encoding/json writes byte for byte: printable
// ASCII with the quote, the backslash and the three characters the default
// HTML escaper rewrites left out.
func jsonSafe(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c > 0x7e || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			return false
		}
	}
	return true
}
