package core

// jsonObject is a JSON object that keeps the order its entries arrive in.
//
// encoding/json sorts the keys of a Go map and PHP does not: json_encode writes
// an array in build order, so a row encoded for a client reads back
// in the order the script assembled it. A map cannot carry that order, which is
// why this type exists in place of a map[string]any.
//
// It is what jsonEncodeValue converts an array into, for the indent path and
// for a Go map's values, both of which reach encoding/json. A document going
// straight to a stream is appended by json_append.go and builds none of these.
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
