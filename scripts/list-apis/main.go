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
	rt := runner.New(io.Discard, runner.Options{SAPI: "cli", RootFS: os.DirFS(".")})
	stdlib.Register(rt)
	stdlib.RegisterFS(rt, ".")
	runner.NewContext().Register(rt)

	doc, err := apidoc.Generate(rt, ".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "list-apis:", err)
		os.Exit(1)
	}
	os.Stdout.WriteString(doc)
}
