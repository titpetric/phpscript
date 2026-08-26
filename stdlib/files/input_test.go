package files_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestPHPInputCLIReadsStdin pins the SAPI split: the cli SAPI maps php://input
// onto stdin, a live stream a script drains once, where a request answers with
// the buffered body however often it is opened. The empty request context is
// registered because that is what the CLI runner does, and it must not shadow
// stdin.
func TestPHPInputCLIReadsStdin(t *testing.T) {
	opts := runner.Options{SAPI: "cli", Stdin: strings.NewReader("piped body")}
	reqCtx := runner.NewContext()

	out := runFSOptions(t, t.TempDir(), &reqCtx, opts, `<?php
echo file_get_contents("php://input"), ";";
echo file_get_contents("php://input") === "" ? "drained" : "rewound";`)

	if want := "piped body;drained"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestPHPInputCLIStream is the same split through a handle: fopen returns the
// live stdin stream under the cli SAPI.
func TestPHPInputCLIStream(t *testing.T) {
	opts := runner.Options{SAPI: "cli", Stdin: strings.NewReader("line in")}

	out := runFSOptions(t, t.TempDir(), nil, opts, `<?php
$h = fopen("php://input", "r");
echo stream_get_contents($h);
fclose($h);`)

	if want := "line in"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
