package helpdocs_test

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/titpetric/phpscript/cmd/phpscript/helpdocs"
)

const document = `Some prose.

| Example | What it does |
|---|---|
| ` + "`phpscript test ./...`" + ` | Runs every fixture. |

Closing prose.
`

// TestRenderMarkdown pins that a redirected --help is the document as it was
// written. Round-tripping it through the table renderer would re-pad every cell
// and rewrite the alignment markers, which is a diff in a file nobody edited.
func TestRenderMarkdown(t *testing.T) {
	var out bytes.Buffer
	if err := helpdocs.Render(&out, document, true); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.String() != document {
		t.Errorf("markdown render =\n%q\nwant the source back", out.String())
	}
}

// TestRenderTerminal covers the ansi path: the table becomes the box-drawing
// table the rest of the tool prints, and everything outside one is printed
// unchanged.
func TestRenderTerminal(t *testing.T) {
	var out bytes.Buffer
	if err := helpdocs.Render(&out, document, false); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := out.String()
	for _, want := range []string{"Some prose.", "Closing prose.", "Example", "Runs every fixture."} {
		if !strings.Contains(got, want) {
			t.Errorf("render is missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "\u256d") || !strings.Contains(got, "\u2502") {
		t.Errorf("render has no box-drawing table:\n%s", got)
	}
	// The separator row is consumed by the parser rather than printed as data.
	if strings.Contains(got, "|---|") {
		t.Errorf("the markdown separator row leaked into the render:\n%s", got)
	}
}

// TestRender pins that a document of prose alone comes out whole, table
// markers that lead nowhere included.
func TestRender(t *testing.T) {
	src := "one\ntwo\n| not a table because nothing follows it\n"
	var out bytes.Buffer
	if err := helpdocs.Render(&out, src, false); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.String() != src {
		t.Errorf("render = %q, want %q", out.String(), src)
	}
}

// TestExamples covers the per-command documents, which is what
// `phpscript <command> --help` prints under its usage line.
func TestExamples(t *testing.T) {
	got := helpdocs.Examples("test", true)
	if !strings.Contains(got, "phpscript test ./...") {
		t.Errorf("test examples are missing the default invocation:\n%s", got)
	}
	if helpdocs.Examples("nosuchcommand", true) != "" {
		t.Error("a command with no document returned something")
	}
}

// TestFS pins that every embedded document is a command's examples and holds a
// table, since that is the whole of what the renderer reads.
func TestFS(t *testing.T) {
	names, err := fs.Glob(helpdocs.FS(), "*.md")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("no example documents are embedded")
	}
	for _, name := range names {
		if !strings.HasPrefix(name, "example.") {
			t.Errorf("%s is not named example.<command>.md", name)
			continue
		}
		data, err := fs.ReadFile(helpdocs.FS(), name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), "| Example | What it does |") {
			t.Errorf("%s has no example table", name)
		}
		if strings.Contains(string(data), "```") {
			t.Errorf("%s holds a code block, which the renderer does not draw", name)
		}
	}
}

// TestWrite covers the whole document: the commands, the shared flags once, and
// a section per command carrying its own flags and its examples.
func TestWrite(t *testing.T) {
	shared := pflag.NewFlagSet("phpscript", pflag.ContinueOnError)
	shared.String("cover", "", "Measure statement coverage")

	own := pflag.NewFlagSet("test", pflag.ContinueOnError)
	own.BoolP("matrix", "m", false, "Run every runtime")

	var out bytes.Buffer
	err := helpdocs.Write(&out, "phpscript", shared, []helpdocs.Command{
		{Name: "test", Title: "Run .phpt test fixtures", Flags: own},
		{Name: "version", Title: "Show version/build information", Flags: pflag.NewFlagSet("version", pflag.ContinueOnError)},
	}, true)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"# phpscript",
		"## Commands",
		"## Global flags",
		"--cover",
		"## test",
		"Run .phpt test fixtures.",
		"-m, --matrix",
		"phpscript test ./...",
		"## version",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("help is missing %q:\n%s", want, got)
		}
	}
	// A command's own flags are its own: the shared set is printed once.
	if strings.Count(got, "Measure statement coverage") != 1 {
		t.Errorf("the shared flags are repeated per command:\n%s", got)
	}
}
