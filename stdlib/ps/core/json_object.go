package core

// jsonObject is a JSON object that keeps the order its entries arrive in.
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

// MarshalJSON writes the entries in insertion order, escaping through json.Marshal where needed.
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
