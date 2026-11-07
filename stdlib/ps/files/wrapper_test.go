package files

import (
	"os"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

func TestRegisterWrapper(t *testing.T) {
	t.Cleanup(func() {
		wrappers.Lock()
		delete(wrappers.byScheme, "scratch")
		wrappers.Unlock()
	})

	rt := runner.New(os.Stdout, runner.Options{})
	if _, ok, _ := openWrapper(rt, "scratch", "r"); ok {
		t.Fatal("an unregistered scheme must not open")
	}

	var open Wrapper = func(_ *runner.Runtime, scheme, mode string) (any, error) {
		if mode == "x" {
			return nil, nil
		}
		return scheme + ":" + mode, nil
	}
	RegisterWrapper("Scratch", open)
	handle, ok, err := openWrapper(rt, "SCRATCH", "w")
	if !ok || err != nil || handle != "SCRATCH:w" {
		t.Fatalf("got handle %v, err %v, ok %v", handle, err, ok)
	}
	// A wrapper refusing the name reads as no wrapper: fopen answers false.
	if handle, ok, err := openWrapper(rt, "scratch", "x"); ok || handle != nil || err != nil {
		t.Fatalf("got handle %v, err %v, ok %v", handle, err, ok)
	}
}
