package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes json_encode and json_decode to stdlib.Register.
func init() {
	runner.RegisterBinding(registerJSON)
}

func registerJSON(rt *runner.Runtime) {
	// json_encode returns the JSON encoding of $value; there is no $flags parameter and an encoding failure raises an error instead of returning false.
	rt.RegisterFunc("json_encode", phpJSONEncode)
	// json_decode parses the JSON in $text; $associative must be true or omitted because there is no stdClass to decode an object into, $depth and $flags are accepted and ignored, and invalid input raises an error instead of returning null.
	rt.RegisterFunc("json_decode", phpJSONDecode)
}

func phpJSONEncode(value any) (any, error) {
	b, err := json.Marshal(jsonEncodeValue(value))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func phpJSONDecode(text string, opts ...any) (any, error) {
	// $associative selects the shape objects decode into. There is no stdClass
	// here, so false is refused rather than answered with the array shape it
	// did not ask for; $depth and $flags are accepted and ignored.
	if len(opts) > 0 && opts[0] != nil && !phpval.Truthy(opts[0]) {
		return nil, errors.New("json_decode(): $associative must be true; there is no stdClass to decode an object into")
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	value, err := jsonDecodeStream(dec)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// jsonDecodeStream reads one value from the token stream.
//
// It reads tokens rather than decoding into a map because a Go map has no
// order and the decoder would hand back the document's keys in a different
// order on every run. PHP preserves the order the object was written in, so a
// decoded object encoded again reads the way it arrived.
func jsonDecodeStream(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return jsonDecodeValue(token), nil
	}

	switch delim {
	case '{':
		out := model.NewArray()
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			value, err := jsonDecodeStream(dec)
			if err != nil {
				return nil, err
			}
			name, _ := key.(string)
			out.Set(name, value)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return out, nil
	case '[':
		out := []any{}
		for dec.More() {
			value, err := jsonDecodeStream(dec)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, fmt.Errorf("json_decode(): unexpected %v", delim)
}

// jsonEncodeValue rewrites the value model into something encoding/json
// understands. Native Go collections are already encodable, so they are only
// walked when they might contain an *model.Array or *model.Object; scalars and
// []string pass through untouched.
func jsonEncodeValue(v any) any {
	switch x := v.(type) {
	case nil, string, bool, int64, int, float64, []string:
		return v
	case *model.Array:
		if x == nil {
			return nil
		}
		if arrayIsList(x) {
			out := make([]any, 0, x.Len())
			x.Range(func(_, v any) bool {
				out = append(out, jsonEncodeValue(v))
				return true
			})
			return out
		}
		out := newJSONObject(x.Len())
		x.Range(func(k, v any) bool {
			out.add(phpval.String(k), jsonEncodeValue(v))
			return true
		})
		return out
	case *model.Object:
		if x == nil {
			return nil
		}
		out := newJSONObject(len(x.Props))
		// Declared fields first, in declaration order, then anything the script
		// added afterwards. Props is a Go map and has no order of its own.
		seen := make(map[string]bool, len(x.Props))
		if x.Class != nil {
			for _, field := range x.Class.Fields {
				if v, ok := x.Props[field.Name]; ok {
					out.add(field.Name, jsonEncodeValue(v))
					seen[field.Name] = true
				}
			}
		}
		if len(seen) < len(x.Props) {
			rest := make([]string, 0, len(x.Props)-len(seen))
			for k := range x.Props {
				if !seen[k] {
					rest = append(rest, k)
				}
			}
			sort.Strings(rest)
			for _, k := range rest {
				out.add(k, jsonEncodeValue(x.Props[k]))
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = jsonEncodeValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = jsonEncodeValue(item)
		}
		return out
	default:
		return v
	}
}

// jsonObject is a JSON object that keeps the order its entries were added in.
//
// encoding/json sorts the keys of a Go map and PHP does not: json_encode writes
// an array in the order it was built, so a row encoded for a client reads back
// in the order the script assembled it. A map cannot carry that order, which is
// why this type exists rather than a map[string]any.
type jsonObject struct {
	keys   []string
	values []any
}

func newJSONObject(size int) *jsonObject {
	return &jsonObject{
		keys:   make([]string, 0, size),
		values: make([]any, 0, size),
	}
}

func (o *jsonObject) add(key string, value any) {
	o.keys = append(o.keys, key)
	o.values = append(o.values, value)
}

// MarshalJSON writes the entries in insertion order.
func (o *jsonObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		encoded, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(encoded)
		buf.WriteByte(':')
		encoded, err = json.Marshal(o.values[i])
		if err != nil {
			return nil, err
		}
		buf.Write(encoded)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// arrayIsList reports whether an *model.Array's keys are the dense int64
// sequence 0..n-1, i.e. whether it is a PHP list.
func arrayIsList(a *model.Array) bool {
	expect := int64(0)
	isList := true
	a.Range(func(k, _ any) bool {
		if i, ok := k.(int64); !ok || i != expect {
			isList = false
			return false
		}
		expect++
		return true
	})
	return isList
}

func jsonDecodeValue(v any) any {
	switch x := v.(type) {
	case []any:
		// A JSON array is positional, so a []any loses nothing and reuses the
		// slice the decoder already allocated.
		for i, item := range x {
			x[i] = jsonDecodeValue(item)
		}
		return x
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return string(x)
	default:
		return v
	}
}
