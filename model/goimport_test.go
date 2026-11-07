package model_test

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestGoImportArea covers the path-to-area mapping an `import` resolves
// through: the acronyms the areas spell their own way, the nesting of a path
// of two segments, and the paths the standard library does not hold.
func TestGoImportArea(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		path  string
		area  string
		alias string
	}{
		{"strings", `Runtime\Go\Strings`, "strings"},
		{"fmt", `Runtime\Go\Fmt`, "fmt"},
		{"net/http", `Runtime\Go\Net\HTTP`, "http"},
		{"encoding/json", `Runtime\Go\Encoding\JSON`, "json"},
		{"database/sql", `Runtime\Go\Database\SQL`, "sql"},
		{"path/filepath", `Runtime\Go\Path\Filepath`, "filepath"},
		{"net/url", `Runtime\Go\Net\URL`, "url"},
		{"unicode/utf8", `Runtime\Go\Unicode\UTF8`, "utf8"},
		{"html/template", `Runtime\Go\HTML\Template`, "template"},
		{"math/big", `Runtime\Go\Math\Big`, "big"},
		{"io/fs", `Runtime\Go\IO\Fs`, "fs"},
	} {
		area, alias, ok := model.GoImportArea(tc.path)
		if !ok {
			t.Errorf("%q resolves to no area", tc.path)
			continue
		}
		if area != tc.area || alias != tc.alias {
			t.Errorf("%q resolves to (%s, %s), want (%s, %s)", tc.path, area, alias, tc.area, tc.alias)
		}
	}
	// A module path, a version element and a spelling no package carries each
	// refuse: the import reaches the standard library alone.
	for _, path := range []string{
		"", "github.com/titpetric/phpscript", "net//http", "math/rand/v2",
		"Net/HTTP", "net/http;", "../strings", "net/http/",
	} {
		if area, alias, ok := model.GoImportArea(path); ok {
			t.Errorf("%q resolves to (%s, %s), want a refusal", path, area, alias)
		}
	}
}
