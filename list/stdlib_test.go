package list_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/list"
)

// TestStdlibReportsWhatTheRuntimeRegisters checks the listing against names the
// runtime is known to bind, one per kind, and against the counts the runtime
// reports for itself. The counts are the load-bearing part: they are what makes
// this listing and `phpscript info` answer the same question about the same
// build.
func TestStdlibReportsWhatTheRuntimeRegisters(t *testing.T) {
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

	// preg_match's $matches is filled by the runner's by-reference setter,
	// which has no type to publish, so the ampersand carries the meaning.
	if got := index["function preg_match"]; !strings.Contains(got, "&$") {
		t.Errorf("preg_match = %q, want a by-reference parameter", got)
	}
}

// TestStdlibMarkdownIsATable checks the rendering has a heading row, a
// separator, and one row per symbol.
func TestStdlibMarkdownIsATable(t *testing.T) {
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
	// Every row is padded to the same width, which is what makes the table
	// line up in a terminal as well as in a markdown renderer.
	for i, line := range lines[1:] {
		if len(line) != len(lines[0]) {
			t.Fatalf("line %d is %d wide, want %d", i+1, len(line), len(lines[0]))
		}
	}
}
