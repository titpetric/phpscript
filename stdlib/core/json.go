package core

import (
	"encoding/json"
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
	// json_decode parses the JSON in $text; objects always decode to arrays (as if $associative were true) and invalid input raises an error instead of returning null.
	rt.RegisterFunc("json_decode", phpJSONDecode)
}

func phpJSONEncode(value any) (any, error) {
	b, err := json.Marshal(jsonEncodeValue(value))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func phpJSONDecode(text string) (any, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return jsonDecodeValue(v), nil
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
		out := make(map[string]any, x.Len())
		x.Range(func(k, v any) bool {
			out[phpval.String(k)] = jsonEncodeValue(v)
			return true
		})
		return out
	case *model.Object:
		if x == nil {
			return nil
		}
		out := make(map[string]any, len(x.Props))
		for k, v := range x.Props {
			out[k] = jsonEncodeValue(v)
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
	case map[string]any:
		// A JSON object becomes an *model.Array. The decoder's map has already
		// lost the document's key order, but an *model.Array at least fixes one
		// order for the value's lifetime, so iterating a decoded object twice
		// renders the same output twice. Handing the map through would make
		// every foreach re-randomise.
		out := model.NewArraySize(len(x))
		for k, v := range x {
			out.Set(k, jsonDecodeValue(v))
		}
		return out
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
