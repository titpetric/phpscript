package core

import (
	"fmt"
	"sort"

	yaml "github.com/goccy/go-yaml"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the YAML pair to stdlib.Register.
func init() {
	runner.RegisterBinding(registerYAML)
}

// registerYAML installs yaml_decode and yaml_encode.
//
// They answer the question json_decode and json_encode answer for the other
// spelling, and they keep their own conversions rather than borrowing the
// JSON ones: json_encode builds an ordered object that only the JSON
// marshaller knows how to write, and handing it to the YAML marshaller
// produced an empty document. The fixture beside this file is what caught
// that.
func registerYAML(rt *runner.Runtime) {
	// yaml_decode parses the YAML in $text into arrays, with a mapping arriving as an array keyed by its field names and a sequence as a list; the keys come back sorted, and invalid input raises an error rather than answering null.
	rt.RegisterFunc("yaml_decode", phpYAMLDecode)
	// yaml_encode returns $value as YAML, writing an array that is a list as a sequence and any other array as a mapping in the order its keys were set.
	rt.RegisterFunc("yaml_encode", phpYAMLEncode)
}

// phpYAMLDecode reads YAML into the shapes json_decode answers with: a
// mapping is an array keyed by its field names, a sequence is a list, and a
// number is an int when it is whole.
func phpYAMLDecode(text string) (any, error) {
	var value any
	if err := yaml.Unmarshal([]byte(text), &value); err != nil {
		return nil, fmt.Errorf("yaml_decode(): %w", err)
	}

	return yamlDecodeValue(value), nil
}

// phpYAMLEncode writes a PHP value as YAML.
func phpYAMLEncode(value any) (any, error) {
	b, err := yaml.Marshal(yamlEncodeValue(value))
	if err != nil {
		return nil, fmt.Errorf("yaml_encode(): %w", err)
	}

	return string(b), nil
}

// yamlDecodeValue turns what the decoder answers into PHP values.
//
// The keys of a mapping are sorted, which is a decision rather than an
// accident: the decoder hands back a Go map, a Go map has no order, and
// reading the same document twice would otherwise build two differently
// ordered arrays. A settings file is read by name, so sorting costs nothing
// and makes the answer the same every time.
//
// A whole number arrives as an int. `max_pages: 400` compared against a
// count has to be one, and a float that happens to be whole would print as
// 400 while behaving as 400.0.
func yamlDecodeValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return yamlMapping(x)
	case map[any]any:
		named := make(map[string]any, len(x))
		for key, val := range x {
			named[fmt.Sprint(key)] = val
		}
		return yamlMapping(named)
	case []any:
		for i, item := range x {
			x[i] = yamlDecodeValue(item)
		}
		return x
	case uint64:
		return int64(x)
	case int:
		return int64(x)
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	}

	return v
}

// yamlMapping builds one array from a decoded mapping, keys in sorted order.
func yamlMapping(in map[string]any) *model.Array {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := model.NewArraySize(len(in))
	for _, key := range keys {
		out.Set(key, yamlDecodeValue(in[key]))
	}

	return out
}

// yamlEncodeValue turns PHP values into what the YAML marshaller writes.
//
// A mapping becomes a yaml.MapSlice rather than a map, so the keys are
// written in the order the array set them: a settings file a script wrote
// should read the way the script built it, and a Go map would shuffle it on
// every run.
func yamlEncodeValue(v any) any {
	switch x := v.(type) {
	case *model.Array:
		if x == nil {
			return nil
		}
		if arrayIsList(x) {
			out := make([]any, 0, x.Len())
			x.Range(func(_, item any) bool {
				out = append(out, yamlEncodeValue(item))
				return true
			})
			return out
		}
		out := make(yaml.MapSlice, 0, x.Len())
		x.Range(func(key, item any) bool {
			out = append(out, yaml.MapItem{Key: phpval.String(key), Value: yamlEncodeValue(item)})
			return true
		})
		return out
	case *model.Object:
		if x == nil {
			return nil
		}
		out := make(yaml.MapSlice, 0, x.Len())
		x.Range(func(name string, item any) bool {
			out = append(out, yaml.MapItem{Key: name, Value: yamlEncodeValue(item)})
			return true
		})
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = yamlEncodeValue(item)
		}
		return out
	}

	return v
}
