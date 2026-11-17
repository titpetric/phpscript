package errors_test

import (
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
)

// TestRegister covers the constructor a script throws and catches.
func TestRegister(t *testing.T) {
	gotest.Run(t, `Runtime\Go\Errors`, []gotest.Case{
		{Src: `use Runtime\Go\Errors; try { throw Errors\New("boom"); } catch (error $e) { echo $e; }`, Want: "boom"},
	})
}
