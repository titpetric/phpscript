package bindings_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// TestLinked covers the gate on what a binding may bind: a package this build
// links answers, and a package it does not refuses, whatever go holds.
func TestLinked(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"net/http", "encoding/json", "database/sql", "strings", "slices", "fmt"} {
		if !bindings.Linked(path) {
			t.Errorf("%s is bound and does not read as linked", path)
		}
	}
	// A package the standard library holds and this build does not link, an
	// internal tree, and a module path.
	for _, path := range []string{
		"net/rpc", "go/ast", "testing", "runtime/trace",
		"internal/abi", "crypto/internal/fips140", "github.com/titpetric/gozero",
	} {
		if bindings.Linked(path) {
			t.Errorf("%s reads as linked", path)
		}
	}
}

// TestLinkedPackages covers the list itself: sorted for reading, free of the
// trees no binding reaches, and a copy the caller cannot write through.
func TestLinkedPackages(t *testing.T) {
	t.Parallel()
	list := bindings.LinkedPackages()
	if !slices.IsSorted(list) {
		t.Error("the list is not sorted")
	}
	for _, path := range list {
		if strings.Contains(path, "internal") {
			t.Errorf("%s is an internal tree", path)
		}
	}
	list[0] = "written"
	if bindings.LinkedPackages()[0] == "written" {
		t.Error("the list is the caller's to write")
	}
}
