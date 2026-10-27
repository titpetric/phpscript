package main

import (
	"strings"
	"testing"
)

// TestGenerate covers the wiring the listing depends on. The generator reads a
// runtime, and what it names is what a script can call: an
// installer left out of generate publishes a reference with a whole area
// missing, and the committed docs/reference/extensions/implemented-apis.md
// would read as though those functions do not exist.
//
// The root is the module root, two directories up, because the generator reads
// doc comments and parameter names out of the Go source tree below it.
func TestGenerate(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	if err := generate(&out, "../.."); err != nil {
		t.Fatalf("generate: %v", err)
	}

	document := out.String()
	if document == "" {
		t.Fatal("generate wrote nothing")
	}

	// One name per installer the command wires in: a core binding, a file
	// system one that only RegisterFS installs, and a superglobal that only
	// the request context registers.
	for _, want := range []string{"strlen", "file_get_contents", "getallheaders"} {
		if !strings.Contains(document, want) {
			t.Errorf("the listing does not name %s, so an installer is missing from generate", want)
		}
	}
}
