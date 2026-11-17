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

// The go runtime's composite literals keep the source spelling: the bracketed
// slice, the braced map and the typed forms beside them.
func TestGoCompositeLiteralsKeepTheirSpelling(t *testing.T) {
	t.Parallel()
	in := "<?php\n\nuse Runtime\\Go;\n\n$list = [1, 2, 3];\n$user = {\"name\": \"ada\"};\n$names = []string{\"ada\"};\n$ages = map[string]int{\"ada\": 36};\n$empty = [];\n$none = {};\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("formatting a go literal changed the source:\n%s\nwant:\n%s", out, in)
	}
}

// TestGoRuntimeFileFormats covers a file the php parser refuses: one import
// per line, a braced map and a typed literal, all printed back as written.
func TestGoRuntimeFileFormats(t *testing.T) {
	t.Parallel()
	in := "<?php\n\nimport \"net/http\";\nimport \"fmt\";\n\n$m = {\n\t\"a\": 1,\n\t\"b\": 2,\n\t\"c\": 3,\n};\n$rows = [][]string{[\"a\"]};\necho http.StatusOK;\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("formatting a go runtime file changed it:\n%s\nwant:\n%s", out, in)
	}
}

// TestGoInferredLiteralPrintsItsInnerType covers the dims-only literal: the
// formatter writes the elided element type back.
func TestGoInferredLiteralPrintsItsInnerType(t *testing.T) {
	t.Parallel()
	in := "<?php\n\nuse Runtime\\Go;\n\n$grid = [][]{{\"a\"}};\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `$grid = [][]{[]{"a"}};`) {
		t.Fatalf("the inferred literal printed as:\n%s", out)
	}
	again, err := formatter.Source(out)
	if err != nil || again != out {
		t.Fatalf("inferred literal is not idempotent: %v\nfirst: %q\nsecond: %q", err, out, again)
	}
}

// The go runtime's `&` and `*` print as written, on a read and on an
// assignment through the pointer.
func TestGoPointerOperatorsKeepTheirSpelling(t *testing.T) {
	t.Parallel()
	in := "<?php\n\nuse Runtime\\Go;\n\n$p = &$a;\n$v = *$p;\n*$p = 2;\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("formatting a go pointer changed the source:\n%s\nwant:\n%s", out, in)
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
