package main

import (
	"testing"
)

// TestLoadPackage covers the symbol table of a standard library package: the
// function, type and constant names, and the methods each type declares.
func TestLoadPackage(t *testing.T) {
	t.Parallel()
	pkg, err := loadPackage("net/http")
	if err != nil {
		t.Fatalf("loadPackage: %v", err)
	}
	for _, name := range []string{"StatusText", "StatusOK", "ServeMux", "Request"} {
		if !pkg.has(name) {
			t.Errorf("net/http holds no %q", name)
		}
	}
	if !pkg.hasMethod("ServeMux", "Handle") {
		t.Error("ServeMux declares no Handle method")
	}
	if pkg.hasMethod("ServeMux", "Use") {
		t.Error("ServeMux declares Use, so the wrapper's own method would link")
	}
}

// TestLoadPackageMissing covers a path the toolchain does not hold: a row it
// names stays plain, and no link reaches a page that does not exist.
func TestLoadPackageMissing(t *testing.T) {
	t.Parallel()
	if _, err := loadPackage("github.com/titpetric/phpscript"); err == nil {
		t.Error("loadPackage accepted a module path")
	}
}
