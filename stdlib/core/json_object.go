package core

// jsonObject is a JSON object that keeps the order its entries arrive in.
//
// encoding/json sorts the keys of a Go map and PHP does not: json_encode writes
// an array in build order and a row encoded for a client reads back in the
// order the script assembled it. A map cannot carry that order and this type
// exists in place of a map[string]any.
//
// jsonEncodeValue converts an array into one, for the indent path and for a Go
// map's values, both of which reach encoding/json. json_append.go appends a
// document going straight to a stream and builds none of these.
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
// an estimate of the document. It appends a key or a scalar that needs no
// escaping as itself; everything else defers to json.Marshal and the bytes
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
