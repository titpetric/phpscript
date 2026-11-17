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
	runner.RegisterArea(`Runtime\PS\Core\Yaml`, registerYAML)
}

// registerYAML installs yaml_decode and yaml_encode with their own conversions, not JSON's.
func registerYAML(rt *runner.Runtime) {
	// yaml_decode parses the YAML in $text into arrays: a mapping arrives as an
	// array keyed by its field names and a sequence as a list. The keys come
	// back sorted, and invalid input raises an error where php answers null.
	rt.RegisterFunc("yaml_decode", phpYAMLDecode)
	// yaml_encode returns $value as YAML, writing an array that is a list as
	// a sequence and any other array as a mapping in key order.
	rt.RegisterFunc("yaml_encode", phpYAMLEncode)
}

// phpYAMLDecode reads YAML into the shapes json_decode answers with.
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

// yamlDecodeValue turns decoded YAML into PHP values, sorting mapping keys and keeping whole numbers int.
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

// yamlEncodeValue turns PHP values into marshaller input, preserving array key order.
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
