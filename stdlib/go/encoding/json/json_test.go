package json_test

import (
	"bytes"
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/stdlib/go/encoding/json"
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

// TestNewEncoder covers the encoder writing a php array with json_encode's spelling.
func TestNewEncoder(t *testing.T) {
	var buf bytes.Buffer
	var enc *json.Encoder = json.NewEncoder(&buf)

	row := model.NewArray()
	row.Set("a", int64(1))
	if err := enc.Encode(row); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got, want := buf.String(), "{\"a\":1}\n"; got != want {
		t.Errorf("Encode = %q, want %q", got, want)
	}
}
