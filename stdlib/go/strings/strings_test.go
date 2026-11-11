package strings_test

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
	"github.com/titpetric/phpscript/runner"
	gostrings "github.com/titpetric/phpscript/stdlib/go/strings"
)

// TestRegister covers the namespace through a script: the import installs it
// and the alias qualifies the names. Go's answers come back as php values.
func TestRegister(t *testing.T) {
	gotest.Run(t, `Runtime\Go\Strings`, []gotest.Case{
		{Src: `use Runtime\Go\Strings; echo implode("|", Strings\Fields("  a b   c "));`, Want: "a|b|c"},
		{Src: `use Runtime\Go\Strings; echo implode("|", Strings\Split("a,b,c", ","));`, Want: "a|b|c"},
		{Src: `use Runtime\Go\Strings; var_dump(Strings\Contains("seafood", "foo"), Strings\HasPrefix("go", "g"), Strings\HasSuffix("go", "x"));`, Want: "bool(true)\nbool(true)\nbool(false)\n"},
		{Src: `use Runtime\Go\Strings; echo Strings\Index("chicken", "ken"), Strings\LastIndex("go gopher", "go"), Strings\Count("cheese", "e");`, Want: "433"},
		{Src: `use Runtime\Go\Strings; echo Strings\Repeat("ab", 3), Strings\ToUpper("abc"), Strings\TrimSpace("  x "), Strings\TrimPrefix("prefix-a", "prefix-");`, Want: "abababABCxa"},
		{Src: `use Runtime\Go\Strings; try { Strings\Repeat("x", -1); } catch (RuntimeException $e) { echo $e->getMessage(); }`, Want: "strings: negative Repeat count"},
		{Src: `use Runtime\Go\Strings; try { Strings\Repeat("abc", PHP_INT_MAX); } catch (RuntimeException $e) { echo $e->getMessage(); }`, Want: "strings: Repeat output too large"},
		{Src: `echo function_exists('Runtime\Go\Strings\Fields') ? "carried" : "absent";`, Want: "absent"},
	})
}

// TestRegisterInstallsOnImportOnly pins Register adding the names to a runtime,
// under a namespace the registry answers for when an import names it.
func TestRegisterInstallsOnImportOnly(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{})
	gostrings.Register(rt)
	if !rt.FunctionExists(gostrings.Namespace + `\Fields`) {
		t.Error("Register left Fields undefined")
	}
	if _, ok := runner.NamespaceInstaller(gostrings.Namespace); !ok {
		t.Error("the namespace is not registered")
	}
}
