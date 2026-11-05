package maps_test

import (
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
)

// TestRegister covers each function through a script and the order the keys
// keep.
func TestRegister(t *testing.T) {
	gotest.Run(t, `Runtime\Go\Maps`, []gotest.Case{
		{Src: `use Runtime\Go\Maps; echo implode(",", Maps\Keys(["b" => 1, "a" => 2]));`, Want: "b,a"},
		{Src: `use Runtime\Go\Maps; echo implode(",", Maps\Values(["b" => 1, "a" => 2]));`, Want: "1,2"},
		{Src: `use Runtime\Go\Maps; $a = ["x" => 1]; $b = Maps\Clone($a); $b["x"] = 2; echo $a["x"], $b["x"];`, Want: "12"},
		{Src: `use Runtime\Go\Maps; var_dump(Maps\Equal(["a" => 1], ["a" => 1]), Maps\Equal(["a" => 1], ["a" => 2]));`, Want: "bool(true)\nbool(false)\n"},
		{Src: `use Runtime\Go\Maps; $m = Maps\Copy(["a" => 1, "b" => 2], ["b" => 3, "c" => 4]); echo json_encode($m);`, Want: `{"a":1,"b":3,"c":4}`},
	})
}
