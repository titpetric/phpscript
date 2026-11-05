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

	var open Wrapper = func(_ *runner.Runtime, mode string) (any, error) { return mode, nil }
	RegisterWrapper("Scratch", open)
	handle, ok, err := openWrapper(rt, "SCRATCH", "w")
	if !ok || err != nil || handle != "w" {
		t.Fatalf("got handle %v, err %v, ok %v", handle, err, ok)
	}
}
