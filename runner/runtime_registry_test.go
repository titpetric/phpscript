package runner

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

// TestRuntimeSymbolTables covers what the introspection functions read. Each
// answers a sorted snapshot, because php guarantees no order and a diagnostic
// that reads differently per run is one nothing can diff.
func TestRuntimeSymbolTables(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})
	rt.RegisterFunc("host_shim", func(args ...any) any { return nil })
	rt.RegisterConstructor("Storage", func() any { return nil })

	program, err := rt.Load(`<?php
interface Greets { public function hello(); }
class Site { public function hello() { return "hei"; } }
function declared_in_php() { return 1; }
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	internal, user := rt.DefinedFunctions()
	if !slices.Contains(internal, "host_shim") {
		t.Errorf("DefinedFunctions internal = %v, want the registered shim among them", internal)
	}
	if !slices.Contains(user, "declared_in_php") {
		t.Errorf("DefinedFunctions user = %v, want the php declaration among them", user)
	}
	if slices.Contains(internal, "declared_in_php") {
		t.Error("a php declaration is reported as internal")
	}
	if !slices.IsSorted(internal) || !slices.IsSorted(user) {
		t.Error("DefinedFunctions answered an unsorted snapshot")
	}

	classes := rt.DeclaredClasses()
	for _, want := range []string{"Site", "Storage", "stdClass"} {
		if !slices.Contains(classes, want) {
			t.Errorf("DeclaredClasses = %v, want %s among them", classes, want)
		}
	}
	if !slices.IsSorted(classes) {
		t.Error("DeclaredClasses answered an unsorted snapshot")
	}

	if _, ok := rt.LookupFunc("host_shim"); !ok {
		t.Error("LookupFunc did not find the registered shim")
	}
	if _, ok := rt.LookupFunc("nothing_registered_this"); ok {
		t.Error("LookupFunc found a name nothing registered")
	}
	if _, ok := rt.LookupConstructor("Storage"); !ok {
		t.Error("LookupConstructor did not find the registered constructor")
	}
	if _, ok := rt.LookupConstructor("Site"); ok {
		t.Error("LookupConstructor found a php class, which has no Go constructor")
	}

	// Method lookup is php's, which is case-insensitive, and answers only for
	// an interpreted class: a caller reflects over a host-backed one itself.
	if !rt.MethodExists("Site", "HELLO") {
		t.Error("MethodExists is case-sensitive, want php's case-insensitive lookup")
	}
	if rt.MethodExists("Site", "absent") || rt.MethodExists("Storage", "Get") {
		t.Error("MethodExists answered for a method the class does not declare")
	}

	declared, err := rt.InterfaceExists("Greets", false)
	if err != nil {
		t.Fatal(err)
	}
	if !declared {
		t.Error("InterfaceExists did not find a declared interface")
	}
	absent, err := rt.InterfaceExists("Absent", false)
	if err != nil {
		t.Fatal(err)
	}
	if absent {
		t.Error("InterfaceExists found an interface nothing declares")
	}
}

// TestRuntime_SetConst covers a constant registered from Go, which is visible
// in every scope the way define() is and unlike a global, which php confines to
// the global one.
func TestRuntime_SetConst(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})
	rt.SetConst("HOST_NAME", "phpscript")

	if value, defined := rt.Const("HOST_NAME"); !defined || value != "phpscript" {
		t.Fatalf("Const(HOST_NAME) = %v, %v, want the registered value", value, defined)
	}

	program, err := rt.Load(`<?php function inside() { echo HOST_NAME; } inside();`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "phpscript" {
		t.Errorf("output = %q, want the constant read inside a function", got)
	}

	// A second registration replaces the value, the call a host swapping one
	// between programs makes.
	rt.SetConst("HOST_NAME", "replaced")
	if value, _ := rt.Const("HOST_NAME"); value != "replaced" {
		t.Errorf("Const(HOST_NAME) = %v after a second SetConst, want the new value", value)
	}
}

// TestRuntime_RegisterShutdown covers the callbacks php runs at the end of a
// program, in registration order, so a registered flush reaches the output of
// a script that already finished. The run consumes each, so the next program
// on a pooled runtime does not run the previous one's.
func TestRuntime_RegisterShutdown(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})

	var ran []string
	rt.RegisterShutdown(func() any { ran = append(ran, "first"); return nil })
	rt.RegisterShutdown(func() any { ran = append(ran, "second"); return nil })

	program, err := rt.Load(`<?php echo "ran";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.Join(ran, ","); got != "first,second" {
		t.Errorf("the callbacks ran %q, want them in registration order", got)
	}
	if got := out.String(); got != "ran" {
		t.Errorf("output = %q, want the program's own text", got)
	}

	ran = nil
	if err := rt.Run(program); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(ran) != 0 {
		t.Errorf("the callbacks ran again on the next program: %v", ran)
	}

	// An error a callback reports reaches the host, because a shutdown that
	// could not finish is a run that did not.
	rt.RegisterShutdown(func() (any, error) { return nil, errors.New("the flush failed") })
	if err := rt.Run(program); err == nil {
		t.Error("a failing shutdown callback did not reach the host")
	}
}

// TestRuntime_PutEnv covers the copy-on-write over the frozen environment:
// a write never lands in the snapshot FreezeStdlib took, and a reset restores
// the snapshot whole.
func TestRuntime_PutEnv(t *testing.T) {
	rt := New(io.Discard, Options{Env: []string{"KEPT=host"}})
	rt.FreezeStdlib()

	rt.PutEnv("ADDED", "x")
	if rt.Env["ADDED"] != "x" || rt.Env["KEPT"] != "host" {
		t.Fatalf("after PutEnv, Env = %v", rt.Env)
	}
	rt.UnsetEnv("KEPT")
	if _, ok := rt.Env["KEPT"]; ok {
		t.Fatal("UnsetEnv left the variable in place")
	}

	rt.ResetSession(io.Discard, nil)
	if _, ok := rt.Env["ADDED"]; ok {
		t.Fatal("a PutEnv write survived the reset")
	}
	if rt.Env["KEPT"] != "host" {
		t.Fatal("the reset did not restore the frozen environment")
	}
}

// TestRuntime_OnReset covers the reset hooks: each registered one runs once
// per reset, so a binding's own state leaves with the program's.
func TestRuntime_OnReset(t *testing.T) {
	rt := New(io.Discard, Options{})
	var calls int
	rt.OnReset(func() { calls++ })
	rt.ResetSession(io.Discard, nil)
	if calls != 1 {
		t.Fatalf("the reset ran the hook %d times, want 1", calls)
	}
}
