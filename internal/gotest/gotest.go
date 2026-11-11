// Package gotest runs the scripts the Runtime\Go namespace tests share: a
// script per case on a runtime that holds the default surface, Runtime\PS. The
// import in the script installs the namespace under test, and RunAreas names
// the areas the runtime installs: Runtime\None is the language with no binding.
package gotest

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// Case is a script and the output it echoes.
type Case struct {
	Src  string
	Want string
}

// Run reports a case whose output differs from the one it echoes.
func Run(t *testing.T, namespace string, cases []Case) {
	t.Helper()
	RunAreas(t, namespace, nil, cases)
}

// RunAreas installs the named areas in place of the default surface.
func RunAreas(t *testing.T, namespace string, areas []string, cases []Case) {
	t.Helper()
	for _, tc := range cases {
		var out strings.Builder
		rt := runner.New(&out, runner.Options{Areas: areas})
		stdlib.Register(rt)
		program, err := rt.Load("<?php " + tc.Src)
		if err != nil {
			t.Errorf("%s: parse %q: %v", namespace, tc.Src, err)
			continue
		}
		if err := rt.Run(program); err != nil {
			t.Errorf("%s: run %q: %v", namespace, tc.Src, err)
			continue
		}
		if out.String() != tc.Want {
			t.Errorf("%s: %q\n got  %q\n want %q", namespace, tc.Src, out.String(), tc.Want)
		}
	}
}
