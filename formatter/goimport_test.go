package formatter_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/formatter"
)

// A file that imports a go package round-trips: the statement prints as the
// path it named, and every member of it as the dotted spelling, where the
// parser resolved both to the area that binds the package.
func TestGoImportKeepsItsSpelling(t *testing.T) {
	t.Parallel()
	in := "<?php\n\nuse Runtime\\Go;\n\nimport \"net/http\";\nimport \"encoding/json\";\n\n$req = http.NewRequest(\"GET\", $url);\n$body = json.Marshal($req);\necho http.StatusOK;\n$rows = make([]http.Request, 2);\n$one = new http.Request;\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("formatting a go import changed the source:\n%s\nwant:\n%s", out, in)
	}
}

// A file that writes the area name itself prints the import's spelling, the
// way php's own `use` shortens a name the file imported.
func TestGoImportShortensTheAreaName(t *testing.T) {
	t.Parallel()
	in := "<?php\n\nuse Runtime\\Go;\n\nimport \"strings\";\n\necho Runtime\\Go\\Strings\\ToUpper(\"a\");\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "strings.ToUpper(\"a\")") {
		t.Fatalf("the area name did not print as the import bound it:\n%s", out)
	}
}
