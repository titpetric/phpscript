package runner

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestRegisterNamespace covers the registry and the install. The registry is
// process-wide and append-only, so the namespace a test registers is one no
// binding package claims, and its installer touches nothing but its own name.
//
// A nil installer and an empty name are dropped: both would reach a runtime
// mid-install, after a program had already resolved part of its calls.
func TestRegisterNamespace(t *testing.T) {
	RegisterNamespace("Test\\Area", func(rt *Runtime) {
		rt.RegisterFunc("test_area_probe", func() string { return "installed" })
	})
	if _, ok := NamespaceInstaller("Test\\Area"); !ok {
		t.Fatal("the registered installer does not resolve")
	}
	// PHP resolves a name in any case and so does the import.
	if _, ok := NamespaceInstaller("\\test\\area"); !ok {
		t.Error("the installer does not resolve case-insensitively, or through a leading separator")
	}
	if _, ok := NamespaceInstaller("Test\\Missing"); ok {
		t.Error("an unregistered namespace resolves to an installer")
	}

	RegisterNamespace("Test\\Dropped", nil)
	if _, ok := NamespaceInstaller("Test\\Dropped"); ok {
		t.Error("a nil installer was registered")
	}
	RegisterNamespace("", func(*Runtime) {})
	if _, ok := NamespaceInstaller(""); ok {
		t.Error("an unnamed namespace was registered")
	}
}

// TestUseNamespace covers what an import does to a runtime: nothing until it
// names a contributed namespace, the installer once when it does, and nothing
// a second time. The surface a runtime carries before the import is the
// guarantee the mechanism exists for.
func TestUseNamespace(t *testing.T) {
	installs := 0
	RegisterNamespace("Test\\Counted", func(rt *Runtime) {
		installs++
		rt.RegisterFunc("test_counted_probe", func() string { return "installed" })
	})

	rt := New(&strings.Builder{}, Options{})
	if rt.FunctionExists("test_counted_probe") {
		t.Fatal("a bare runtime answers for the binding before anything imported it")
	}
	if rt.UseNamespace("Test\\Nothing") {
		t.Error("an unregistered namespace reported an install")
	}
	if !rt.UseNamespace("Test\\Counted") {
		t.Fatal("the import reported no install")
	}
	if !rt.FunctionExists("test_counted_probe") {
		t.Error("the binding does not resolve after the import")
	}
	if rt.UseNamespace("test\\counted") {
		t.Error("a second import of the same namespace reported an install")
	}
	if installs != 1 {
		t.Errorf("the installer ran %d times, want 1", installs)
	}
}

// TestUseImports covers the program-level seam Run uses, which is what serves
// the bytecode engine: it compiles and runs a program whole and reaches no
// statement interpreter, so the install happens before either engine starts.
// A statement that is not an import is passed over.
func TestUseImports(t *testing.T) {
	RegisterNamespace("Test\\Program", func(rt *Runtime) {
		rt.RegisterFunc("test_program_probe", func() string { return "installed" })
	})

	rt := New(&strings.Builder{}, Options{})
	rt.useImports([]model.Stmt{
		&model.Echo{},
		&model.Use{Imports: []model.UseImport{{Path: "Vendor\\Thing"}, {Path: "Test\\Program"}}},
	})
	if !rt.FunctionExists("test_program_probe") {
		t.Error("the binding does not resolve after the program's imports ran")
	}
}
