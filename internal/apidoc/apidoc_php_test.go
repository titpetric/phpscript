package apidoc

import (
	"strings"
	"testing"
)

// phpClass is a class with one of everything a definition prints: a
// description, a cited standard library symbol, a satisfied interface, a
// documented property, an undocumented one, and a method.
func phpClass() Class {
	return Class{
		Name:    "Database",
		Comment: []string{"Database is the database client scripts query."},
		Params:  []Param{{Name: "name", Type: "string"}},
		Binds:   "github.com/titpetric/phpscript/stdlib/database",
		Uses:    Citation{Package: "github.com/titpetric/phpscript/stdlib/database", Name: "Database", Go: "database.Database"},
		Implements: []Citation{
			{Package: "fmt", Name: "Stringer", Go: "fmt.Stringer", URL: "https://pkg.go.dev/fmt#Stringer"},
		},
		Fields: []Field{
			{Name: "id", GoName: "ID", Type: "string"},
			{
				Name:    "is_readonly",
				GoName:  "IsReadonly",
				Type:    "bool",
				Comment: []string{"IsReadonly restricts the client to statements that only read.", "", "PHP reads and writes it as `$db->is_readonly`."},
			},
		},
		Methods: []Method{
			{
				Name:    "get_all",
				GoName:  "GetAll",
				Params:  []Param{{Name: "query", Type: "string"}, {Name: "args", Type: "mixed", Variadic: true}},
				Returns: "mixed",
				Comment: []string{"get_all returns all result rows."},
			},
		},
	}
}

// TestClass_PHP covers the definition a class prints: the fields before the
// functions, every member's godoc as a docblock, and no body anywhere.
func TestClass_PHP(t *testing.T) {
	t.Parallel()
	want := `/**
 * Database is the database client scripts query.
 *
 * Binds github.com/titpetric/phpscript/stdlib/database package, uses Database.
 * Implements [fmt.Stringer](https://pkg.go.dev/fmt#Stringer).
 */
class Database implements fmt.Stringer
{
    public string $id;

    /**
     * IsReadonly restricts the client to statements that only read.
     *
     * PHP reads and writes it as ` + "`$db->is_readonly`" + `.
     */
    public bool $is_readonly;

    public function __construct(string $name);

    /**
     * get_all returns all result rows.
     */
    public function get_all(string $query, mixed ...$args): mixed;
}
`
	if got := phpClass().PHP(); got != want {
		t.Errorf("PHP() =\n%s\nwant\n%s", got, want)
	}
}

// TestClassPHPFieldsBeforeFunctions pins the order of the two halves. A reader
// looking for what a class holds reads it before what it does, which is the
// order `go doc` prints a Go type in.
func TestClassPHPFieldsBeforeFunctions(t *testing.T) {
	t.Parallel()
	out := phpClass().PHP()
	property := strings.Index(out, "public bool $is_readonly;")
	constructor := strings.Index(out, "public function __construct")
	method := strings.Index(out, "public function get_all")
	switch {
	case property < 0 || constructor < 0 || method < 0:
		t.Fatalf("a member is missing from the definition:\n%s", out)
	case property > constructor:
		t.Error("a property prints after the constructor, want every property first")
	case constructor > method:
		t.Error("the constructor prints after a method, want it first of the functions")
	}
}

// TestClassPHPPrintsNoBody pins where a declaration ends. The implementation is
// Go and no PHP a reader could open, and a declaration ends at the semicolon,
// with no body for a reader to look inside.
func TestClassPHPPrintsNoBody(t *testing.T) {
	t.Parallel()
	for _, line := range strings.Split(phpClass().PHP(), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "public ") {
			continue
		}
		if !strings.HasSuffix(trimmed, ";") {
			t.Errorf("declaration does not end with a semicolon: %q", trimmed)
		}
		if strings.Contains(trimmed, "{") {
			t.Errorf("declaration carries a body: %q", trimmed)
		}
	}
}

// TestClassPHPWithoutMembers covers a class with no property to print: it
// prints no property section and no prose about the absence. HTTP\Client is
// the one in this tree, and a sentence saying it holds nothing is a sentence
// about the binding and not about the class.
func TestClassPHPWithoutMembers(t *testing.T) {
	t.Parallel()
	c := Class{
		Name:    "HTTP\\Client",
		Comment: []string{"HTTP\\Client sends requests."},
		Binds:   "github.com/titpetric/phpscript/stdlib/http",
		Uses:    Citation{Package: "net/http", Name: "Client", Go: "http.Client", URL: "https://pkg.go.dev/net/http#Client"},
		Params:  []Param{{Name: "options", Type: "mixed"}},
	}
	want := `/**
 * HTTP\Client sends requests.
 *
 * Binds github.com/titpetric/phpscript/stdlib/http package, uses [net/http.Client](https://pkg.go.dev/net/http#Client).
 */
class HTTP\Client
{
    public function __construct(mixed $options);
}
`
	if got := c.PHP(); got != want {
		t.Errorf("PHP() =\n%s\nwant\n%s", got, want)
	}
}

// TestClass_Describe covers the description: the godoc of the bound Go type,
// then the citation of that type and of the interfaces it satisfies. The
// renderer adds no further line.
func TestClass_Describe(t *testing.T) {
	t.Parallel()
	got := phpClass().Describe()
	want := []string{
		"Database is the database client scripts query.",
		"",
		"Binds github.com/titpetric/phpscript/stdlib/database package, uses Database.",
		"Implements [fmt.Stringer](https://pkg.go.dev/fmt#Stringer).",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Describe() = %q, want %q", got, want)
	}
}

// TestClassDescribeWithoutGodoc covers a class the index carries no comment
// for. The citation stands alone and the renderer invents no sentence around
// it.
func TestClassDescribeWithoutGodoc(t *testing.T) {
	t.Parallel()
	c := Class{Name: "RuntimeException", Binds: "x", Uses: Citation{Package: "x", Name: "Exception", Go: "x.Exception"}}
	got := c.Describe()
	if len(got) != 1 || got[0] != "Binds x package, uses Exception." {
		t.Errorf("Describe() = %q, want the citation alone", got)
	}
}

// TestClassImplementsClause covers the clause the class header carries. A
// bound type satisfying none of the interface set prints no clause: no
// placeholder and no prose about the absence.
func TestClassImplementsClause(t *testing.T) {
	t.Parallel()
	if got := (Class{}).implementsClause(); got != "" {
		t.Errorf("implementsClause() = %q, want none", got)
	}
	c := Class{Implements: []Citation{
		{Name: "Throwable", Go: "runner.Throwable", PHP: "Throwable"},
		{Name: "Stringer", Go: "fmt.Stringer"},
	}}
	if got, want := c.implementsClause(), " implements Throwable, fmt.Stringer"; got != want {
		t.Errorf("implementsClause() = %q, want %q", got, want)
	}
}

// TestClass_Rename covers the copy an alias prints under. The SPL exception
// names share one constructor and one entry, and a caller asking about
// RuntimeException reads that name in the header.
func TestClass_Rename(t *testing.T) {
	t.Parallel()
	c := Class{Name: "Error", Aliases: []string{"RuntimeException", "TypeError"}}
	renamed := c.Rename("RuntimeException")
	if got, want := renamed.Name, "RuntimeException"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
	if renamed.Aliases != nil {
		t.Errorf("Aliases = %q, want none: the name asked about is not its own alias", renamed.Aliases)
	}
	if got, want := c.Name, "Error"; got != want {
		t.Errorf("the original class is now named %q, want %q", got, want)
	}
}

// TestWriteDocblock covers the docblock form. A blank godoc line carries no
// trailing space, which a formatter would strip and a diff would then report.
func TestWriteDocblock(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeDocblock(&b, nil, "    ")
	if b.String() != "" {
		t.Errorf("an undocumented member printed %q, want nothing", b.String())
	}
	writeDocblock(&b, []string{"one", "", "two"}, "    ")
	want := "    /**\n     * one\n     *\n     * two\n     */\n"
	if got := b.String(); got != want {
		t.Errorf("writeDocblock =\n%q\nwant\n%q", got, want)
	}
}
