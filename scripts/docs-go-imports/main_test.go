package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/runner"
)

// TestGenerate covers the page: the generator replaces both markers and every
// name the runtime binds reaches the document. Each import path is the one an
// import statement writes.
func TestGenerate(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := generate(&out, "../.."); err != nil {
		t.Fatalf("generate: %v", err)
	}
	doc := out.String()

	if strings.Contains(doc, "{{") {
		t.Error("generated page still holds a template marker")
	}
	for _, want := range []string{
		"| [`errors`](https://pkg.go.dev/errors)",
		"| [`net/http`](https://pkg.go.dev/net/http)",
		"### `Runtime\\Go\\Net\\HTTP` (`net/http`)",
		"### `Runtime\\Go`",
		"### `Runtime\\PS\\Compat`",
		"[`ErrNoRows`](https://pkg.go.dev/database/sql#ErrNoRows)",
		"[`ServeMux`](https://pkg.go.dev/net/http#ServeMux)",
		"[`Mux::handle`](https://pkg.go.dev/net/http#ServeMux.Handle)",
		"`NewEncoder(object): object`",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("generated page does not contain %q", want)
		}
	}
	for _, unwanted := range []string{
		"https://pkg.go.dev/net/http#Aborted",
		"https://pkg.go.dev/net/http#IgnoreUserAbort",
		"https://pkg.go.dev/net/http#ServeMux.Use",
	} {
		if strings.Contains(doc, unwanted) {
			t.Errorf("generated page links %q, which the package does not declare", unwanted)
		}
	}

	for _, s := range list.Stdlib(runner.Options{Runtime: goRuntime}) {
		if s.Area == "" {
			continue
		}
		name := strings.TrimPrefix(s.Name, s.Area+`\`)
		if !strings.Contains(doc, "`"+name+"`") {
			t.Errorf("generated page omits %s %q", s.Kind, name)
		}
	}
}

// TestGroupsOrder covers the row order: importable areas by path first, then
// the areas an import does not reach.
func TestGroupsOrder(t *testing.T) {
	t.Parallel()
	groups := groups([]list.Symbol{
		{Kind: "function", Name: `Runtime\Go\len`, Area: `Runtime\Go`},
		{Kind: "function", Name: `Runtime\Go\Strings\Fields`, Area: `Runtime\Go\Strings`},
		{Kind: "constant", Name: `Runtime\Go\Database\SQL\ErrNoRows`, Area: `Runtime\Go\Database\SQL`},
	})
	if got, want := len(groups), 3; got != want {
		t.Fatalf("groups hold %d areas, want %d", got, want)
	}
	if got, want := groups[0].path, "database/sql"; got != want {
		t.Errorf("first area is %q, want %q", got, want)
	}
	if got, want := groups[1].path, "strings"; got != want {
		t.Errorf("second area is %q, want %q", got, want)
	}
	if got, want := groups[2].area, `Runtime\Go`; got != want {
		t.Errorf("last area is %q, want %q", got, want)
	}
}
