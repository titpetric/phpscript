package strconv_test

import (
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
)

// TestRegister covers both conversions and the error Atoi raises.
func TestRegister(t *testing.T) {
	gotest.Run(t, `Runtime\Go\Strconv`, []gotest.Case{
		{Src: `use Runtime\Go\Strconv; echo Strconv\Atoi("42") + 1;`, Want: "43"},
		{Src: `use Runtime\Go\Strconv; echo Strconv\Itoa(42);`, Want: "42"},
		{Src: `use Runtime\Go\Strconv; try { Strconv\Atoi("x"); } catch (Exception $e) { echo "bad"; }`, Want: "bad"},
	})
}
