package time_test

import (
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
)

// TestRegister covers a string duration, the spelling a script writes.
func TestRegister(t *testing.T) {
	gotest.Run(t, `Runtime\Go\Time`, []gotest.Case{
		{Src: `use Runtime\Go\Time; Time\Sleep("1ms"); echo "awake";`, Want: "awake"},
	})
}
