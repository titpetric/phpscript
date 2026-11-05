package json_test

import (
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
)

// TestRegister covers the three functions through a script, including the
// error a malformed document raises.
func TestRegister(t *testing.T) {
	gotest.Run(t, `Runtime\Go\Encoding\JSON`, []gotest.Case{
		{Src: `use Runtime\Go\Encoding\JSON; echo JSON\Marshal(["a" => 1, "b" => [true, null]]);`, Want: `{"a":1,"b":[true,null]}`},
		{Src: `use Runtime\Go\Encoding\JSON; $v = JSON\Unmarshal('{"x":[1,2]}'); echo $v["x"][1];`, Want: "2"},
		{Src: `use Runtime\Go\Encoding\JSON; var_dump(JSON\Valid('{"a":1}'), JSON\Valid('{'));`, Want: "bool(true)\nbool(false)\n"},
		{Src: `use Runtime\Go\Encoding\JSON; try { JSON\Unmarshal('{'); } catch (Exception $e) { echo "bad"; }`, Want: "bad"},
		{Src: `echo function_exists('json_encode') ? "global" : "none";`, Want: "global"},
	})
}
