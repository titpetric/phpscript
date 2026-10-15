package runner

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/model"
)

// TestIncludeFunc covers the resolver a host installs for include and require.
// It is what keeps the runner free of a file system: the hook answers with a
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
// the script sees the hook's value where PHP would see the file's, and the file
// beside it is never parsed. Nothing is installed until the script includes it,
// so the PHP semantics hold.
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

	// The path is cleaned at registration, so the spellings a script may write
	// for one file reach the same hook.
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

// TestRuntime_RecordError covers the failure a script has no throw to catch:
// one that happened around the script and no statement in it, a request body
// refused for its size being the case it covers. It records and
// returns, so the host sees it and PHP execution is not unwound.
func TestRuntime_RecordError(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})

	var seen []error
	rt.OnError(func(err error) { seen = append(seen, err) })
	if !rt.HasErrorHandler() {
		t.Fatal("HasErrorHandler is false after OnError installed one")
	}

	refused := errors.New("the request body is over the limit")
	rt.RecordError(refused)
	if len(seen) != 1 || !errors.Is(seen[0], refused) {
		t.Fatalf("the handler received %v, want the one error", seen)
	}

	// A nil error is nothing to report, so it never reaches the handler.
	rt.RecordError(nil)
	if len(seen) != 1 {
		t.Errorf("the handler received %d errors, want the nil to have been dropped", len(seen))
	}

	// Nothing about it is visible to a script: the program runs to its end.
	var out strings.Builder
	rt.ResetSession(&out, nil)
	program, err := rt.Load(`<?php echo "ran";`)
	if err != nil {
		t.Fatal(err)
	}
	rt.RecordError(errors.New("recorded before the run"))
	if err := rt.Run(program); err != nil {
		t.Errorf("a recorded error ended the run: %v", err)
	}
	if got := out.String(); got != "ran" {
		t.Errorf("output = %q, want the program to have run", got)
	}
}

// TestRuntime_SPLAutoload covers PHP's default autoloader: the class name
// lowercased, its namespace separators read as directories, and class.php
// looked for under each include_path entry in order.
//
// It answers nil for a class it cannot find, because a failed autoload is not
// an error in PHP: the next loader in the queue gets its turn, and the missing
// class is reported by the call that needed it.
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

	// The second include_path entry is searched too, so the list is a list.
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

// TestRuntime_UnregisterAutoloader covers how a loader is matched for removal,
// which is the way PHP matches it: by the function, object and method it names
// and not by identity, because each `array($this, "loadClass")` a script writes
// is a fresh array and comparing those would never remove anything.
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

// phpThrowable stands in for the binding-side exception value, which knows the
// PHP class a script constructed it as.
type phpThrowable struct {
	class string
	msg   string
}

func (e *phpThrowable) Error() string { return e.msg }

func (e *phpThrowable) ThrowableClass() string { return e.class }

func TestMatchCatchTypeUsesTheClassName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		thrown   string
		declared string
		want     bool
	}{
		// The name is compared, so a clause takes the class it names.
		{"Exception", "Exception", true},
		{"Exception", "Throwable", true},
		{"Exception", "", true},
		{"Exception", "\\Exception", true},
		{"RuntimeException", "RuntimeException", true},
		{"RuntimeException", "runtimeexception", true},

		// Exception and Error split on the name's suffix.
		{"RuntimeException", "Exception", true},
		{"NotFound", "Exception", true},
		{"TypeError", "Exception", false},
		{"ValueError", "Exception", false},
		{"Error", "Exception", false},
		{"TypeError", "Error", true},
		{"MyError", "Error", true},
		{"RuntimeException", "Error", false},

		// A union takes the error when any alternative does.
		{"RuntimeException", "LogicException|RuntimeException", true},
		{"RuntimeException", "LogicException|TypeError", false},

		// There is no hierarchy. Every row here would pass under one and must
		// not here: see docs/design.md.
		{"InvalidArgumentException", "LogicException", false},
		{"OverflowException", "RuntimeException", false},
		{"ArgumentCountError", "TypeError", false},
		{"DivisionByZeroError", "ArithmeticError", false},
		{"RuntimeException", "LogicException", false},
	}

	for _, tt := range tests {
		err := &phpThrowable{class: tt.thrown, msg: "boom"}
		if got := matchCatchType(tt.declared, err); got != tt.want {
			t.Errorf("catch (%s) of %s: got %v, want %v", tt.declared, tt.thrown, got, tt.want)
		}
	}
}

// TestMatchCatchTypeTakesAnyClauseForANonThrowable is the contract a host
// binding is written against: an error a Go function returned, and a panic
// recovered at the call boundary, belong to no PHP class, so the catch a script
// already wrote has to reach them whichever throwable class it names.
func TestMatchCatchTypeTakesAnyClauseForANonThrowable(t *testing.T) {
	t.Parallel()
	for _, err := range []error{
		errors.New("plain go error"),
		fmt.Errorf("wrapped: %w", errors.New("inner")),
		&HostPanicError{Callable: "Storage.Get", Value: "boom"},
	} {
		for _, declared := range []string{"Exception", "Error", "Throwable", "RuntimeException", "TypeError"} {
			if !matchCatchType(declared, err) {
				t.Errorf("catch (%s) of %v: got false, want true", declared, err)
			}
		}
	}
}

// TestRunnerThrowablesNameTheirClass covers the throwables the runner raises
// itself. Each names a PHP class, which keeps an engine error out of
// the clauses PHP would not route it to.
func TestRunnerThrowablesNameTheirClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err      error
		declared string
		want     bool
	}{
		{NewRuntimeException("out of memory", 0), "RuntimeException", true},
		{NewRuntimeException("out of memory", 0), "Exception", true},
		{NewRuntimeException("out of memory", 0), "Error", false},
		{&ArithmeticError{Message: "negative shift"}, "ArithmeticError", true},
		{&ArithmeticError{Message: "negative shift"}, "Error", true},
		{&ArithmeticError{Message: "negative shift"}, "Exception", false},
		{&TypeError{Name: "explode"}, "TypeError", true},
		{&TypeError{Name: "explode"}, "Error", true},
		{&TypeError{Name: "explode"}, "Exception", false},
		{&ArgumentCountError{Name: "strlen"}, "ArgumentCountError", true},
		{&ArgumentCountError{Name: "strlen"}, "Error", true},
		{&ArgumentCountError{Name: "strlen"}, "Exception", false},
	}

	for _, tt := range tests {
		if got := matchCatchType(tt.declared, tt.err); got != tt.want {
			t.Errorf("catch (%s) of %T: got %v, want %v", tt.declared, tt.err, got, tt.want)
		}
	}
}

// TestNoInheritanceAtRuntime is the executable form of a design decision:
// phpscript has no OOP, and `extends` confers nothing. docs/design.md refuses
// it; read that before changing this test.
func TestNoInheritanceAtRuntime(t *testing.T) {
	t.Parallel()
	// model.Class may record what a declaration listed, so
	// Implements is allowed: it is a list of names a class checks
	// against, and no member arrives through it. A parent is different, and
	// forbidden: a class that has one would start acquiring members it did not
	// declare.
	for _, name := range []string{"Parent", "Extends", "Parents", "Super", "Base"} {
		if _, found := reflect.TypeOf(model.Class{}).FieldByName(name); found {
			t.Fatalf("model.Class has a %s field: phpscript has no inheritance, `extends` is recorded on model.ClassDecl for the formatter only. See docs/design.md.", name)
		}
	}

	// A class declaring `extends RuntimeException` is its own class and nothing
	// else, to a catch clause and to instanceof alike.
	thrown := newObjectError(model.NewObject(&model.Class{Name: "NotFound"}))
	if matchCatchType("RuntimeException", thrown) {
		t.Error("catch (RuntimeException) took a NotFound: `extends` must confer nothing")
	}
	if !matchCatchType("NotFound", thrown) {
		t.Error("catch (NotFound) did not take a NotFound")
	}

	obj := model.NewObject(&model.Class{Name: "NotFound"})
	if phpInstanceOf(obj, "RuntimeException") {
		t.Error("instanceof RuntimeException was true for a NotFound: `extends` must confer nothing")
	}
	if !phpInstanceOf(obj, "notfound") {
		t.Error("instanceof is class-name equality and is case-insensitive")
	}
}
