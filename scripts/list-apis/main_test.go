package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/internal/apidoc"
)

// TestGenerate covers the wiring the listing depends on. The generator reads a
// runtime. The names it publishes are the calls a script can make: an
// installer left out of generate publishes a reference with a whole area
// missing. The committed docs/reference/extensions/implemented-apis.md would
// then read as though those functions do not exist.
//
// The root is the module root, two directories up, because the generator reads
// doc comments and parameter names from the Go source tree below.
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
	// system one that only RegisterFS installs, plus a superglobal that only
	// the request context registers.
	for _, want := range []string{"strlen", "file_get_contents", "getallheaders"} {
		if !strings.Contains(document, want) {
			t.Errorf("the listing does not name %s, so an installer is missing from generate", want)
		}
	}
}

// TestIndex covers the committed doc index against the source tree it comes
// from. A shipped binary answers `phpscript list --classes` from the index. A
// comment edited without regenerating the index ships the old prose beneath
// the new code.
func TestIndex(t *testing.T) {
	t.Parallel()
	root := "../.."
	docs, err := apidoc.ScanDocs(listRuntime(root), root)
	if err != nil {
		t.Fatalf("scan docs: %v", err)
	}
	if len(docs.Classes) == 0 {
		t.Fatal("the scan found no documented class")
	}
	encoded, err := encodeIndex(docs)
	if err != nil {
		t.Fatalf("encode index: %v", err)
	}

	committed, err := os.ReadFile(filepath.Join(root, apidoc.IndexPath))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !bytes.Equal(committed, encoded) {
		t.Errorf("%s is stale; regenerate with atkins test:introspection", apidoc.IndexPath)
	}
}
