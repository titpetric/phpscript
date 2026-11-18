package apidoc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestCleanComment covers the text a comment loses on the way to the reference:
// the marker, one space per line, a blank line at either end, plus a `//nolint`
// or build directive. That directive instructs a tool, not prose.
func TestCleanComment(t *testing.T) {
	t.Parallel()
	group := &ast.CommentGroup{List: []*ast.Comment{
		{Text: "// upper returns $string uppercased."},
		{Text: "//"},
		{Text: "//\tupper(\"a\") // A"},
		{Text: "//nolint:gocyclo"},
	}}

	got := cleanComment(group)
	want := []string{"upper returns $string uppercased.", "", "\tupper(\"a\") // A"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("cleanComment = %q, want %q", got, want)
	}
}

// TestDeclCommentRewritesTheSymbol covers the godoc fallback: a registration
// without a comment takes the declaration's, with the Go name the
// comment opens on replaced by the name a script types. A comment opening on
// something else travels unchanged.
func TestDeclCommentRewritesTheSymbol(t *testing.T) {
	t.Parallel()
	src := `package fake

// NamedShim reports nothing in particular.
func NamedShim(a any) bool { return false }

// Reports nothing, and names no symbol.
func Other(a any) bool { return false }
`
	file, err := parser.ParseFile(token.NewFileSet(), "fake.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		decl int
		php  string
		want string
	}{
		{decl: 0, php: "named", want: "named reports nothing in particular."},
		{decl: 1, php: "other", want: "Reports nothing, and names no symbol."},
	} {
		fn, ok := file.Decls[test.decl].(*ast.FuncDecl)
		if !ok {
			t.Fatalf("declaration %d is not a func", test.decl)
		}
		if got := strings.Join(declComment(fn, test.php), "\n"); got != test.want {
			t.Errorf("declComment = %q, want %q", got, test.want)
		}
	}
}
