package path_test

import (
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
)

// TestRegister covers the lexical functions through a script, including the one
// that answers a Go error as a thrown exception.
func TestRegister(t *testing.T) {
	gotest.Run(t, `Runtime\Go\Path`, []gotest.Case{
		{Src: `use Runtime\Go\Path; echo Path\Base("/a/b.txt"), Path\Dir("/a/b.txt"), Path\Ext("/a/b.txt");`, Want: "b.txt/a.txt"},
		{Src: `use Runtime\Go\Path; echo Path\Clean("a//b/../c"), Path\Join("a", "b", "../c");`, Want: "a/ca/c"},
		{Src: `use Runtime\Go\Path; var_dump(Path\IsAbs("/x"), Path\Match("*.go", "main.go"));`, Want: "bool(true)\nbool(true)\n"},
		{Src: `use Runtime\Go\Path; try { Path\Match("[", "x"); } catch (Exception $e) { echo "bad pattern"; }`, Want: "bad pattern"},
	})
}
