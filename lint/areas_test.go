package lint

import (
	"testing"

	"github.com/titpetric/phpscript/parser"
)

// TestFileAcceptsNamesAnImportedAreaBinds covers the names an opt-in area
// installs: a call and a class under an area the file imports resolve, in the
// go spelling and in the php one. The same name under an area no statement
// names is the finding the check exists for.
func TestFileAcceptsNamesAnImportedAreaBinds(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		src  string
		want int
	}{
		{
			name: "the go spelling of the import",
			src:  "<?php\nuse Runtime\\Go;\nimport \"strings\";\necho strings.ToUpper(\"a\");\n",
		},
		{
			name: "the php spelling of the same area",
			src:  "<?php\nuse Runtime\\Go\\Strings;\necho Strings\\ToUpper(\"a\");\n",
		},
		{
			name: "a qualified call under the imported area",
			src:  "<?php\nuse Runtime\\Go\\Strings;\necho Runtime\\Go\\Strings\\ToUpper(\"a\");\n",
		},
		{
			name: "a class the area installs",
			src:  "<?php\nuse Runtime\\Go;\nimport \"net/http\";\n$r = new http.Request;\n",
		},
		{
			name: "a name under an area nothing imports",
			src:  "<?php\necho Runtime\\Go\\Strings\\ToUpper(\"a\");\n",
			want: 1,
		},
		{
			name: "a name no area binds",
			src:  "<?php\nuse Runtime\\Go\\Strings;\necho Strings\\Nothing(\"a\");\n",
			want: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			diags, err := File("areas.php", test.src)
			if err != nil {
				t.Fatalf("File returned an error: %v", err)
			}
			if len(diags) != test.want {
				t.Errorf("got %d diagnostics, want %d: %+v", len(diags), test.want, diags)
			}
		})
	}
}

// TestProgramAreasReadsInstallersOnly covers what counts as an area: a php
// namespace no installer answers for is the registry's business and builds no
// runtime.
func TestProgramAreasReadsInstallersOnly(t *testing.T) {
	t.Parallel()
	prog, err := parser.Parse("<?php\nuse App\\Models\\User;\nuse Runtime\\Go\\Strings;\n")
	if err != nil {
		t.Fatal(err)
	}
	areas := programAreas(prog)
	if len(areas) != 1 || areas[0] != `Runtime\Go\Strings` {
		t.Errorf("programAreas = %v, want the one installer path", areas)
	}
}
