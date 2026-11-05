// Command list-apis generates docs/reference/extensions/implemented-apis.md.
// It builds the same runtime the phpscript CLI runs, reading doc comments and
// parameter names from the Go source tree it runs in, and the listing is what
// a script sees. Run it from the module root:
//
//	go run ./scripts/list-apis > docs/reference/extensions/implemented-apis.md
//	go run ./scripts/list-apis --index
//
// The atkins test:introspection job does both. --index writes the doc index
// `phpscript list --classes` reads out of the binary, at the path
// apidoc.IndexPath names. The command opens that file itself, because a shell
// redirection would truncate the file the compiler is about to embed.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/titpetric/phpscript/internal/apidoc"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

func main() {
	index := flag.Bool("index", false, "Write the embedded doc index instead of the markdown listing")
	flag.Parse()

	run, target := generate, "list-apis"
	if *index {
		run, target = writeIndex, "list-apis --index"
	}
	if err := run(os.Stdout, "."); err != nil {
		fmt.Fprintln(os.Stderr, target+":", err)
		os.Exit(1)
	}
}

// generate writes the listing for the tree at root, the whole of the command,
// kept apart from main so a test can assert the wiring:
// the listing is what a script sees, and a runtime built with one installer
// missing publishes a reference that omits a whole area.
func generate(out io.Writer, root string) error {
	owned := stdlib.Owners(stdlib.Areas(stdlib.All))
	doc, err := apidoc.GenerateAreas(listRuntime(root), root, apidoc.Owners{Funcs: owned.Funcs, Classes: owned.Classes})
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, doc)
	return err
}

// writeIndex writes the doc index for the tree at root to apidoc.IndexPath
// below it, and reports the path it wrote on out.
func writeIndex(out io.Writer, root string) error {
	docs, err := apidoc.ScanDocs(listRuntime(root), root)
	if err != nil {
		return err
	}
	encoded, err := encodeIndex(docs)
	if err != nil {
		return err
	}
	path := filepath.Join(root, apidoc.IndexPath)
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "list-apis: wrote %s, %d classes\n", path, len(docs.Classes))
	return nil
}

// encodeIndex renders the index the way the file holds it, indented and
// newline terminated, and a regenerated file that matches leaves no diff.
func encodeIndex(docs *apidoc.Docs) ([]byte, error) {
	encoded, err := json.MarshalIndent(docs, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// listRuntime builds the runtime both halves of the command read. A runtime
// built with one installer missing publishes a reference that omits a whole
// area.
func listRuntime(root string) *runner.Runtime {
	rt := runner.New(io.Discard, runner.Options{SAPI: "cli", RootFS: os.DirFS(root)})
	stdlib.Register(rt)
	stdlib.RegisterFS(rt, root)
	runner.NewContext().Register(rt)
	return rt
}
