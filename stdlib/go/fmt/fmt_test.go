package fmt_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
	"github.com/titpetric/phpscript/runner"
	gofmt "github.com/titpetric/phpscript/stdlib/go/fmt"
)

// TestSet covers the area through a script: the import installs it, the dotted
// name resolves to it, and php's values arrive as the verbs read them.
func TestSet(t *testing.T) {
	gotest.Run(t, gofmt.Namespace, []gotest.Case{
		{Src: `use Runtime\Go; import "fmt"; echo fmt.Sprintf("%s=%d", "n", 42);`, Want: "n=42"},
		{Src: `use Runtime\Go; import "fmt"; echo fmt.Sprintf("%.2f|%t|%q", 1.5, true, "a");`, Want: `1.50|true|"a"`},
		{Src: `use Runtime\Go; import "fmt"; echo fmt.Sprint("a", 1, 2, "b");`, Want: "a1 2b"},
		{Src: `use Runtime\Go; import "fmt"; echo fmt.Sprintln("a", "b");`, Want: "a b\n"},
		// The area answers under its own name as well, as every area does.
		{Src: `use Runtime\Go\Fmt; echo Fmt\Sprintf("%d", 7);`, Want: "7"},
		// The default runtime carries none of it.
		{Src: `echo function_exists('Runtime\Go\Fmt\Sprintf') ? "carried" : "absent";`, Want: "absent"},
	})
}

// TestNamespace pins the registration an import resolves to.
func TestNamespace(t *testing.T) {
	if gofmt.Namespace != `Runtime\Go\Fmt` {
		t.Errorf("the area is %s", gofmt.Namespace)
	}
	if _, ok := runner.NamespaceInstaller(gofmt.Namespace); !ok {
		t.Error("the area is not registered")
	}
	rt := runner.New(io.Discard, runner.Options{})
	gofmt.Set.Register(rt)
	if !rt.FunctionExists(gofmt.Namespace + `\Sprintf`) {
		t.Error("Sprintf is undefined")
	}
}
