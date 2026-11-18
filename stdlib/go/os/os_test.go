package os_test

import (
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
)

// TestRegister covers Getenv for a held name and for a missing one.
func TestRegister(t *testing.T) {
	t.Setenv("PHPSCRIPT_GO_OS", "value")
	gotest.Run(t, `Runtime\Go\Os`, []gotest.Case{
		{Src: `use Runtime\Go\Os; echo Os\Getenv("PHPSCRIPT_GO_OS");`, Want: "value"},
		{Src: `use Runtime\Go\Os; echo "[" . Os\Getenv("PHPSCRIPT_GO_OS_MISSING") . "]";`, Want: "[]"},
	})
}
