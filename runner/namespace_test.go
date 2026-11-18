package runner

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestRegisterNamespace covers the registry and the install. The registry is
// process-wide and append-only. The namespace a test registers is a name that
// no binding package claims, with an installer that touches that name alone.
//
// RegisterNamespace drops a nil installer and an empty name. Either would
// reach a runtime mid-install, after a program had resolved part of its calls
// already.
func TestRegisterNamespace(t *testing.T) {
	RegisterNamespace("Test\\Area", func(rt *Runtime) {
		rt.RegisterFunc("test_area_probe", func() string { return "installed" })
	})
	if _, ok := NamespaceInstaller("Test\\Area"); !ok {
		t.Fatal("the registered installer does not resolve")
	}
	// PHP resolves a name in any case. The import resolves it the same way.
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

// TestUseNamespace covers the change an import makes to a runtime. An import
// naming no contributed namespace leaves the surface alone. An import naming
// one runs the installer. A second import of the same namespace runs nothing
// further. The surface a runtime holds before the import is the guarantee the
// mechanism exists for.
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

// TestUseImports covers the program-level seam Run uses, needed by the
// bytecode engine. That engine compiles and runs a program whole and reaches
// no statement interpreter. The install happens before either engine starts.
// The loop passes over any statement importing no namespace.
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

// TestUseImportPaths covers three ways a file can name a contributed namespace:
// the namespace; a `use function` import of a symbol inside the namespace; a
// `use const` import. Those name one segment more. A path below a contributed namespace
// installs no deeper than one segment, whichever segment the install reads.
func TestUseImportPaths(t *testing.T) {
	RegisterNamespace("Test\\Symbol", func(rt *Runtime) {
		rt.RegisterFunc("test_symbol_probe", func() string { return "installed" })
	})

	for _, path := range []string{"Test\\Symbol", "Test\\Symbol\\thing", "\\Test\\Symbol"} {
		rt := New(&strings.Builder{}, Options{})
		rt.useImport(path)
		if !rt.FunctionExists("test_symbol_probe") {
			t.Errorf("%s installed nothing", path)
		}
	}
	rt := New(&strings.Builder{}, Options{})
	rt.useImport("Test\\Symbol\\thing\\deeper")
	if rt.FunctionExists("test_symbol_probe") {
		t.Error("an import two segments below the namespace installed it")
	}
}

// TestUseImportReadsAKnownNamespaceOnce covers the import of a registered
// namespace seen twice, as `useImports` before the run and the `use` statement
// during it do: the second pass finds the namespace already installed and does
// not fall back to the namespace above. That parent would install too.
func TestUseImportReadsAKnownNamespaceOnce(t *testing.T) {
	parentInstalls := 0
	RegisterNamespace("Test\\Twice", func(*Runtime) { parentInstalls++ })
	RegisterNamespace("Test\\Twice\\Child", func(*Runtime) {})

	rt := New(nil, Options{})
	rt.useImport("Test\\Twice\\Child")
	rt.useImport("Test\\Twice\\Child")
	if parentInstalls != 0 {
		t.Fatalf("the parent installed %d times from an import of its child", parentInstalls)
	}

	rt.useImport("Test\\Twice\\Child\\fn")
	if parentInstalls != 0 {
		t.Fatalf("a symbol of an installed namespace installed the parent")
	}
	rt.useImport("Test\\Twice\\unknown")
	if parentInstalls != 1 {
		t.Fatalf("a symbol of a namespace falls back to it once: %d installs", parentInstalls)
	}
}
