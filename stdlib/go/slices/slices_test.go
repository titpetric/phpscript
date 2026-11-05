package slices_test

import (
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
)

// TestRegister covers each function through a script, including the type
// strictness of Contains and the error of Max on an empty list.
func TestRegister(t *testing.T) {
	gotest.Run(t, `Runtime\Go\Slices`, []gotest.Case{
		{Src: `use Runtime\Go\Slices; var_dump(Slices\Contains([1, 2, 3], 2), Slices\Contains([1, 2], "2"));`, Want: "bool(true)\nbool(false)\n"},
		{Src: `use Runtime\Go\Slices; echo Slices\Index(["a", "b", "c"], "c"), Slices\Index(["a"], "z");`, Want: "2-1"},
		{Src: `use Runtime\Go\Slices; echo implode(",", Slices\Reverse([1, 2, 3]));`, Want: "3,2,1"},
		{Src: `use Runtime\Go\Slices; $in = [3, 1, 2]; echo implode(",", Slices\Sort($in)), implode(",", $in);`, Want: "1,2,3" + "3,1,2"},
		{Src: `use Runtime\Go\Slices; echo Slices\Max([3, 9, 2]), Slices\Min([3, 9, 2]);`, Want: "92"},
		{Src: `use Runtime\Go\Slices; try { Slices\Max([]); } catch (Exception $e) { echo "empty"; }`, Want: "empty"},
		{Src: `use Runtime\Go\Slices; echo implode(",", Slices\Compact([1, 1, 2, 2, 1]));`, Want: "1,2,1"},
		{Src: `use Runtime\Go\Slices; echo implode(",", Slices\Concat([1], [2, 3]));`, Want: "1,2,3"},
		{Src: `use Runtime\Go\Slices; var_dump(Slices\Equal([1, 2], [1, 2]), Slices\Equal([1], [1, 2]));`, Want: "bool(true)\nbool(false)\n"},
	})
}
