package runner

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/model"
)

// TestIncludeFunc covers the resolver a host installs for include and require.
// The resolver keeps the runner free of a file system: the hook answers with a
// parsed program, so a host serving scripts out of anything at all resolves
// the path itself. An error it reports is the include failing, and nothing
// falls back to reading a file.
func TestIncludeFunc(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})

	included, err := rt.Load(`<?php echo "included";`)
	if err != nil {
		t.Fatal(err)
	}

	var asked []string
	resolver := IncludeFunc(func(path string) (*model.Program, error) {
		asked = append(asked, path)
		if path == "missing.php" {
			return nil, errors.New("the host has no such file")
		}
		return included, nil
	})
	rt.SetIncludeResolver(resolver)

	program, err := rt.Load(`<?php include "lib.php"; echo "after";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); got != "includedafter" {
		t.Errorf("output = %q, want %q", got, "includedafter")
	}
	if len(asked) != 1 || asked[0] != "lib.php" {
		t.Errorf("the resolver received %v, want the one path the script named", asked)
	}

	out.Reset()
	failing, err := rt.Load(`<?php require "missing.php"; echo "unreached";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(failing); err == nil {
		t.Error("a require the resolver refused did not fail the program")
	}
	if got := out.String(); strings.Contains(got, "unreached") {
		t.Errorf("the program continued past a failed require: %q", got)
	}
}

// TestRuntime_RegisterInclude covers a host reimplementing one include target
// in Go. composer's generated vendor/autoload.php is the case it exists for:
// the script sees the hook's value where PHP would see the file's, and the
// runner never parses the file beside it. The hook runs when the script
// includes the path and not before, so the PHP semantics hold.
func TestRuntime_RegisterInclude(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{RootFS: fstest.MapFS{
		"vendor/autoload.php": &fstest.MapFile{Data: []byte(`<?php echo "the file ran";`)},
	}})

	calls := 0
	rt.RegisterInclude("vendor/autoload.php", func() (any, error) {
		calls++
		return nil, nil
	})

	program, err := rt.Load(`<?php require "vendor/autoload.php"; echo "after";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if calls != 1 {
		t.Errorf("the hook ran %d times, want once", calls)
	}
	if got := out.String(); got != "after" {
		t.Errorf("output = %q, want the file not to have been parsed", got)
	}

	// RegisterInclude cleans the path at registration, so the spellings a
	// script may write for one file reach the same hook.
	out.Reset()
	same, err := rt.Load(`<?php require "./vendor/autoload.php";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(same); err != nil {
		t.Fatalf("run: %v", err)
	}
	if calls != 2 {
		t.Errorf("the hook ran %d times, want the second spelling to reach it", calls)
	}

	// An error the hook reports is the include failing, as the file being
	// unreadable would be.
	rt.RegisterInclude("broken.php", func() (any, error) {
		return nil, errors.New("the host cannot build this one")
	})
	broken, err := rt.Load(`<?php require "broken.php"; echo "unreached";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(broken); err == nil {
		t.Error("a hook reporting an error did not fail the program")
	}
}

// TestRuntime_SPLAutoload covers PHP's default autoloader: the class name
// lowercased, its namespace separators read as directories, and class.php
// looked for under each include_path entry in order.
//
// It answers nil for a class it cannot find, because a failed autoload is not
// an error in PHP: the next loader in the queue gets its turn, and the call
// that needed the class reports it missing.
func TestRuntime_SPLAutoload(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{RootFS: fstest.MapFS{
		"lib/app/orders.php": &fstest.MapFile{Data: []byte(`<?php class Orders { public $name = "orders"; }`)},
		"other/widget.php":   &fstest.MapFile{Data: []byte(`<?php class Widget {}`)},
	}})
	rt.SetIncludePath("lib" + string(':') + "other")

	declared := func(name string) bool {
		t.Helper()
		found, err := rt.ClassExists(name, false)
		if err != nil {
			t.Fatalf("ClassExists(%s): %v", name, err)
		}
		return found
	}

	if err := rt.SPLAutoload(`\App\Orders`); err != nil {
		t.Fatalf("SPLAutoload: %v", err)
	}
	if !declared("Orders") {
		t.Error("the class the included file declares is not defined")
	}

	// SPLAutoload searches the second entry too, so the list is a list.
	if err := rt.SPLAutoload("Widget"); err != nil {
		t.Fatalf("SPLAutoload(Widget): %v", err)
	}
	if !declared("Widget") {
		t.Error("a class under the second include_path entry was not found")
	}

	// A class no entry holds is not an error, and defines nothing.
	if err := rt.SPLAutoload("Absent"); err != nil {
		t.Errorf("SPLAutoload of a class nothing declares = %v, want nil", err)
	}
	if declared("Absent") {
		t.Error("a class nothing declares is defined")
	}
}

// TestRuntime_UnregisterAutoloader covers how PHP matches a loader for
// removal: by the function, object and method it names and not by identity,
// because each `array($this, "loadClass")` a script writes is a fresh array
// and comparing those would never remove anything.
func TestRuntime_UnregisterAutoloader(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})

	object := model.NewObject(&model.Class{Name: "Loader"})
	method := func() *model.Array {
		pair := model.NewArray()
		pair.Append(object)
		pair.Append("loadClass")
		return pair
	}

	rt.RegisterAutoloader("spl_autoload", false)
	rt.RegisterAutoloader(method(), false)

	// A second array naming the same object and method is a different array
	// and the same loader.
	if !rt.UnregisterAutoloader(method()) {
		t.Error("a method loader was not matched by the object and method it names")
	}
	if rt.UnregisterAutoloader(method()) {
		t.Error("the method loader was removed twice")
	}

	if !rt.UnregisterAutoloader("spl_autoload") {
		t.Error("a named loader was not matched by its name")
	}
	if rt.UnregisterAutoloader("spl_autoload") {
		t.Error("the named loader was removed twice")
	}
	if rt.UnregisterAutoloader("never_registered") {
		t.Error("a loader nothing registered was reported removed")
	}
}
