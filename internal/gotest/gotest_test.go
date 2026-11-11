package gotest

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestRun covers the helper on a script that needs no namespace.
func TestRun(t *testing.T) {
	Run(t, "none", []Case{{Src: `echo 1 + 1;`, Want: "2"}})
}

// TestRunAreas covers the runtime a test states: the default holds the
// standard library. Runtime\None leaves the language alone, where a binding
// reads as an undefined function.
func TestRunAreas(t *testing.T) {
	cases := []Case{{Src: `echo 1 + 2, strlen("abc");`, Want: "33"}}
	RunAreas(t, "default", nil, cases)
	RunAreas(t, `Runtime\PS`, []string{`Runtime\PS`}, cases)
	RunAreas(t, `Runtime\None`, []string{`Runtime\None`}, []Case{{Src: `echo 1 + 2;`, Want: "3"}})

	rt := runner.New(io.Discard, runner.Options{Areas: []string{`Runtime\None`}})
	stdlib.Register(rt)
	program, err := rt.Load(`<?php echo strlen("abc");`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err == nil {
		t.Error("strlen ran on a runtime with no area")
	}
}
