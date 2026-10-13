package core

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestRegisterEnvironment covers the putenv/getenv pair reading one map on the
// runtime, so a variable a script sets is visible to the same script and to
// nothing in the process around it.
//
// getenv answers false for a name nothing set, which is php's contract and the
// reason a caller compares with === false: the empty string is a value a
// variable can hold. putenv answers true for every call it accepts, and a bare
// "NAME" with no '=' unsets.
func TestRegisterEnvironment(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{})
	registerEnvironment(rt)

	putenv, ok := rt.Callable("putenv")
	if !ok {
		t.Fatal("putenv is not registered")
	}
	getenv, ok := rt.Callable("getenv")
	if !ok {
		t.Fatal("getenv is not registered")
	}

	if got, err := getenv("PHPSCRIPT_TEST_VAR"); err != nil {
		t.Fatalf("getenv: %v", err)
	} else if got != false {
		t.Errorf("getenv() on an unset name = %#v, want false", got)
	}

	if got, err := putenv("PHPSCRIPT_TEST_VAR=1"); err != nil {
		t.Fatalf("putenv: %v", err)
	} else if got != true {
		t.Errorf("putenv() = %#v, want true", got)
	}
	if got, err := getenv("PHPSCRIPT_TEST_VAR"); err != nil {
		t.Fatalf("getenv: %v", err)
	} else if got != "1" {
		t.Errorf("getenv() = %#v, want \"1\"", got)
	}

	if _, err := putenv("PHPSCRIPT_TEST_VAR"); err != nil {
		t.Fatalf("putenv: %v", err)
	}
	if got, err := getenv("PHPSCRIPT_TEST_VAR"); err != nil {
		t.Fatalf("getenv: %v", err)
	} else if got != false {
		t.Errorf("getenv() after the bare-name form = %#v, want false", got)
	}

	if _, err := putenv("PHPSCRIPT_TEST_VAR="); err != nil {
		t.Fatalf("putenv: %v", err)
	}
	if got, err := getenv("PHPSCRIPT_TEST_VAR"); err != nil {
		t.Fatalf("getenv: %v", err)
	} else if got != "" {
		t.Errorf("getenv() on a name set to the empty string = %#v, want \"\"", got)
	}
}
