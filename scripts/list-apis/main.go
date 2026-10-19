// Command list-apis generates docs/reference/extensions/implemented-apis.md.
// It builds the same runtime the phpscript CLI runs, so the listing is what a
// script sees, and reads doc comments and parameter names from the Go source
// tree it runs in. Run it from the module root:
//
//	go run ./scripts/list-apis > docs/reference/extensions/implemented-apis.md
//
// The atkins test:introspection job does exactly that.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/titpetric/phpscript/internal/apidoc"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

func main() {
	if err := generate(os.Stdout, "."); err != nil {
		fmt.Fprintln(os.Stderr, "list-apis:", err)
		os.Exit(1)
	}
}

// generate writes the listing for the tree at root, the whole of the command,
// kept apart from main so a test can assert the wiring:
// the listing is what a script sees, so a runtime built with one installer
// missing publishes a reference that omits a whole area.
func generate(out io.Writer, root string) error {
	rt := runner.New(io.Discard, runner.Options{SAPI: "cli", RootFS: os.DirFS(root)})
	stdlib.Register(rt)
	stdlib.RegisterFS(rt, root)
	runner.NewContext().Register(rt)

	doc, err := apidoc.Generate(rt, root)
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, doc)
	return err
}
