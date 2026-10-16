package core

import (
	"encoding/json"
	"strconv"
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
		if buf, err = appendJSONValue(buf, pair.key); err != nil {
			return nil, err
		}
		buf = append(buf, ':')
		if buf, err = appendJSONValue(buf, pair.value); err != nil {
			return nil, err
		}
	}
	return append(buf, '}'), nil
}

// appendJSONValue appends one value's encoding: the scalar shapes a handler's
// response is made of directly, and json.Marshal for the rest, floats
// included, whose spelling is encoding/json's own.
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
		if jsonSafe(x) {
			buf = append(buf, '"')
			buf = append(buf, x...)
			return append(buf, '"'), nil
		}
	case int64:
		return strconv.AppendInt(buf, x, 10), nil
	case int:
		return strconv.AppendInt(buf, int64(x), 10), nil
	}
	encoded, err := json.Marshal(v)
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
