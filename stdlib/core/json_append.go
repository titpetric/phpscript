package core

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// This file appends a PHP value's JSON encoding to a buffer. json_encode()
// returns that buffer as a string and JSON\Encoder writes it. The bytes are the ones
// encoding/json writes for the same document: the shapes handled here are the
// ones it writes verbatim, and everything else defers to it.
//
// Each walk spells its loop out and calls Range directly, taking no function
// value as a parameter. A closure appending to buf captures buf, and a closure
// reaching a function value the compiler cannot see through escapes to the heap
// along with everything it captured: that is an allocation per collection, paid
// again per row of a list of rows.

// appendJSONValue appends one value's encoding: the scalar and collection
// shapes a handler's response carries directly, and json.Marshal for the
// rest, floats included, whose spelling is encoding/json's own.
//
// The collections append in place, so a document reaches the stream without
// building the jsonEncodeValue tree first and walking it by reflection after.
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
			return appendJSONArrayList(x, buf)
		}
		return appendJSONArrayPairs(x, buf)
	case *model.Object:
		if x == nil {
			return append(buf, "null"...), nil
		}
		return appendJSONObject(x, buf)
	case model.Collection:
		// A view over request data, keyed by name, so an object. The read
		// decodes the request behind it.
		if x == nil {
			return append(buf, "null"...), nil
		}
		return appendJSONCollection(x, buf)
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

// appendJSONArrayList appends a PHP list as a JSON array.
func appendJSONArrayList(a *model.Array, buf []byte) ([]byte, error) {
	buf = slices.Grow(buf, a.Len()*8+2)
	buf = append(buf, '[')
	first := true
	var failure error
	a.Range(func(_, value any) bool {
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

// appendJSONArrayPairs appends a keyed PHP array as a JSON object, in the
// insertion order, which is the order the script built it and the order php
// writes it in.
//
// Keys and no Range, because a keyed array answers its insertion order slice
// itself and allocates nothing for it, and a JSON response consists of keyed
// arrays.
func appendJSONArrayPairs(a *model.Array, buf []byte) ([]byte, error) {
	keys := a.Keys()
	buf = slices.Grow(buf, len(keys)*24+2)
	buf = append(buf, '{')
	for i, key := range keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		var err error
		if buf, err = appendJSONString(buf, phpval.String(key)); err != nil {
			return nil, err
		}
		buf = append(buf, ':')
		value, _ := a.Get(key)
		if buf, err = appendJSONValue(buf, value); err != nil {
			return nil, err
		}
	}
	return append(buf, '}'), nil
}

// appendJSONObject appends an object's properties as a JSON object, in
// declaration order.
func appendJSONObject(o *model.Object, buf []byte) ([]byte, error) {
	buf = slices.Grow(buf, o.Len()*24+2)
	buf = append(buf, '{')
	first := true
	var failure error
	o.Range(func(name string, value any) bool {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		if buf, failure = appendJSONString(buf, name); failure != nil {
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

// appendJSONCollection appends a request data view as a JSON object, keyed by
// name in the order the view reports.
func appendJSONCollection(c model.Collection, buf []byte) ([]byte, error) {
	buf = slices.Grow(buf, c.Len()*24+2)
	buf = append(buf, '{')
	first := true
	var failure error
	c.Range(func(key, value any) bool {
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
// no any, because an any parameter boxes the key, and a string does not fit an
// interface word: that is one allocation per key of every object a response
// carries.
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
