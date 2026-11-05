// Package json binds go's encoding/json as the Runtime\Go\Encoding\JSON
// namespace. A script imports it with `use Runtime\Go\Encoding\JSON;` and calls
// `JSON\Marshal($value)`; the default runtime carries none of these names.
package json

import (
	gojson "encoding/json"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/ps/core"
)

// Namespace is the name a script imports.
const Namespace = model.GoNamespace + `\Encoding\JSON`

func init() {
	runner.RegisterNamespace(Namespace, Register)
}

func name(fn string) string { return Namespace + `\` + fn }

// Register installs the Runtime\Go\Encoding\JSON functions on a runtime that imported them.
func Register(rt *runner.Runtime) {
	// Runtime\Go\Encoding\JSON\Marshal returns the JSON encoding of $v.
	rt.RegisterFunc(name("Marshal"), func(v any) (string, error) { return core.JSONEncode(v) })
	// Runtime\Go\Encoding\JSON\Unmarshal decodes the JSON in $data, objects as arrays.
	rt.RegisterFunc(name("Unmarshal"), func(data string) (any, error) { return core.JSONDecode(data) })
	// Runtime\Go\Encoding\JSON\Valid reports whether $data is valid JSON.
	rt.RegisterFunc(name("Valid"), func(data string) bool { return gojson.Valid([]byte(data)) })
}
