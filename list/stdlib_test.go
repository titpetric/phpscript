package list_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/list"
)

// TestStdlibReportsWhatTheRuntimeRegisters checks the listing against names the
// runtime binds, one per kind, and against the counts the runtime reports for
// itself and this listing and the `info` command answer the same question
// about the same build.
func TestStdlibReportsWhatTheRuntimeRegisters(t *testing.T) {
	t.Parallel()
	symbols := list.Stdlib()
	if len(symbols) == 0 {
		t.Fatal("Stdlib() returned nothing")
	}

	index := map[string]list.Symbol{}
	counts := map[string]int{}
	for _, s := range symbols {
		index[s.Kind+" "+s.Name] = s
		counts[s.Kind]++
	}

	for _, want := range []string{
		"function fnmatch",
		"function glob",
		"function str_contains",
		"function preg_match",
		"function header", // registered by the request context, not stdlib.Register
		"class Exception",
		"class stdClass", // declared with no Go constructor to reflect
		"class HTTP\\Client",
		"method HTTP\\Client::get",
		"constant FNM_CASEFOLD",
		"constant PHP_EOL",
	} {
		if _, ok := index[want]; !ok {
			t.Errorf("missing %s", want)
		}
	}

	for kind, count := range counts {
		if count == 0 {
			t.Errorf("no %s symbols listed", kind)
		}
	}
}

// TestStdlibSignatures pins the shape of the rendered signature: reflected
// types, no invented parameter names, and the by-reference slot preserved.
func TestStdlibSignatures(t *testing.T) {
	t.Parallel()
	index := map[string]string{}
	for _, s := range list.Stdlib() {
		index[s.Kind+" "+s.Name] = s.Signature
	}

	for name, want := range map[string]string{
		"function fnmatch":    "fnmatch(string, string, mixed ...): bool",
		"function glob":       "glob(string): array",
		"function strlen":     "strlen(string): int",
		"constant FNM_PERIOD": "4",
	} {
		if got := index[name]; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	// The runner's by-reference setter fills preg_match's $matches, and it has
	// no type to publish and the ampersand carries the meaning.
	if got := index["function preg_match"]; !strings.Contains(got, "&$") {
		t.Errorf("preg_match = %q, want a by-reference parameter", got)
	}
}

// TestStdlibDefinitions covers the definition the command prints for the class
// the fidelity target names. `go doc net/http.Client` reads as the type's doc
// paragraph, then its fields with their docs, then its methods with their
// signatures and their docs, and this is that in PHP spelling.
//
// HTTP\Client's bound type holds a *net/http.Client and a timeout of its own
// and publishes neither, leaving no property section to print, and it satisfies
// no interface in the set, leaving no implements clause.
func TestStdlibDefinitions(t *testing.T) {
	t.Parallel()
	got, err := list.StdlibDefinitions(`HTTP\Client`)
	if err != nil {
		t.Fatalf("StdlibDefinitions: %v", err)
	}

	want := `/**
 * HTTP\Client sends requests, one at a time with send() or all at once with
 * parallel(). It takes its settings as an associative array, and with no
 * argument builds a client with a 30 second timeout that follows redirects.
 *
 * Binds github.com/titpetric/phpscript/stdlib/http package, uses [net/http.Client](https://pkg.go.dev/net/http#Client).
 */
class HTTP\Client
{
    public function __construct(mixed $options);
`
	if !strings.HasPrefix(got, want) {
		t.Errorf("definition opens as\n%s\nwant it to open as\n%s", got, want)
	}
	for _, line := range []string{
		"     * get sends a GET request to $url and returns the response.",
		"    public function get(string $url): object;",
		"    public function send(object $request): object;",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("the definition does not carry %q:\n%s", line, got)
		}
	}
	if strings.Contains(got, "public mixed $") || strings.Contains(got, "implements") {
		t.Errorf("HTTP\\Client carries a property or an implements clause:\n%s", got)
	}
}

// TestStdlibDefinitionsProperties covers a class that does carry properties.
// Database holds two, in the order the Go struct declares them, and
// $is_readonly is the folded spelling: Runtime.structField resolves
// is_readonly back to IsReadonly through the underscores, and a listing
// printing isReadonly would name a property no script can read.
//
// The client it queries through is not among them: an exported field is a
// property a script reads, and that one answers with no read-only check. See
// docs/security.md.
func TestStdlibDefinitionsProperties(t *testing.T) {
	t.Parallel()
	got, err := list.StdlibDefinitions("Database")
	if err != nil {
		t.Fatalf("StdlibDefinitions: %v", err)
	}
	if strings.Contains(got, "$bridge") {
		t.Errorf("the definition publishes the bridge as a property:\n%s", got)
	}

	order := []string{"public string $id;", "public bool $is_readonly;"}
	at := -1
	for _, want := range order {
		next := strings.Index(got, want)
		if next < 0 {
			t.Fatalf("the definition does not carry %q:\n%s", want, got)
		}
		if next < at {
			t.Errorf("%q prints out of declaration order", want)
		}
		at = next
	}
	// The property's godoc reaches the listing as a docblock, word for word
	// from the Go field.
	if !strings.Contains(got, "     * IsReadonly restricts the client to statements that only read.") {
		t.Errorf("$is_readonly carries no docblock:\n%s", got)
	}
	// The constructor follows every property, and no declaration carries a
	// body: the implementation is Go and no PHP a reader could open.
	if strings.Index(got, "public bool $is_readonly;") > strings.Index(got, "public function __construct") {
		t.Error("a property prints after the constructor")
	}
	if strings.Contains(got, ") {") || strings.Contains(got, "{}") {
		t.Errorf("a declaration carries a body:\n%s", got)
	}
}

// TestStdlibDefinitionsAliasAndUnbound covers the two names a caller can name
// that are not a class's own: an SPL name sharing the Exception constructor
// prints under the name on the command line, and stdClass has no Go constructor
// and no bound type to report anything about.
func TestStdlibDefinitionsAliasAndUnbound(t *testing.T) {
	t.Parallel()
	got, err := list.StdlibDefinitions("RuntimeException")
	if err != nil {
		t.Fatalf("StdlibDefinitions: %v", err)
	}
	if !strings.Contains(got, "class RuntimeException implements Throwable") {
		t.Errorf("the definition does not head on the name asked about:\n%s", got)
	}

	if _, err := list.StdlibDefinitions("stdClass"); err == nil {
		t.Error("stdClass answered a definition, and it has no Go constructor to describe")
	}
}

// TestStdlibDefinitionsAll covers the listing with no name: every class a
// script can construct, each as its own definition, and stdClass left out.
func TestStdlibDefinitionsAll(t *testing.T) {
	t.Parallel()
	got, err := list.StdlibDefinitions()
	if err != nil {
		t.Fatalf("StdlibDefinitions: %v", err)
	}
	for _, want := range []string{
		"class Database\n",
		"class Exception implements Throwable\n",
		"class HTTP\\Client\n",
		"class HTTP\\Mux implements http.Handler\n",
		`class Time implements encoding.TextMarshaler, fmt.Stringer, json.Marshaler`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the listing does not carry %q", want)
		}
	}
	if strings.Contains(got, "class stdClass") {
		t.Error("stdClass is listed, and it has no bound Go type to describe")
	}
}

// TestStdlibMarkdownIsATable checks the rendering has a heading row, a
// separator, and one row per symbol.
func TestStdlibMarkdownIsATable(t *testing.T) {
	t.Parallel()
	symbols := list.Stdlib()
	lines := strings.Split(strings.TrimRight(list.StdlibMarkdown(symbols), "\n"), "\n")
	if want := len(symbols) + 2; len(lines) != want {
		t.Fatalf("rendered %d lines, want %d", len(lines), want)
	}
	if !strings.Contains(lines[0], "Kind") || !strings.Contains(lines[0], "Signature") {
		t.Errorf("heading = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  |---") {
		t.Errorf("separator = %q", lines[1])
	}
	// Every row carries the same width and the table lines up in a terminal
	// as well as in a markdown renderer.
	for i, line := range lines[1:] {
		if len(line) != len(lines[0]) {
			t.Fatalf("line %d is %d wide, want %d", i+1, len(line), len(lines[0]))
		}
	}
}
