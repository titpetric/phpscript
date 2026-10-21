package lint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/lint"
)

func TestFlatstackLinterCompatibility(t *testing.T) {
	t.Parallel()
	compatibleSrc := `<?php
	$a = 1 + 2;
	echo $a;
	?>`

	diag, err := lint.FlatstackFile("compatible.php", compatibleSrc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diag.Message != "[flatstack compatible] 100% compatible with flatstack bytecode engine" {
		t.Fatalf("expected compatible message, got %s", diag.Message)
	}

	// An anonymous class is what the bytecode engine has no representation
	// for: it carries a class name where one carries its declaration.
	unsupportedSrc := `<?php
	$a = new class { public $x = 1; };
	?>`

	diag, err = lint.FlatstackFile("unsupported.php", unsupportedSrc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diag.Message == "[flatstack compatible] 100% compatible with flatstack bytecode engine" {
		t.Fatalf("expected unsupported diagnostic for an anonymous class, got %s", diag.Message)
	}
}

// TestFlatstackPaths covers the path-expanding half of the compatibility
// check: one diagnostic per file the patterns selected, in filename order, so
// a caller pointing at a directory reads a verdict for every file in it and
// never one for the directory.
func TestFlatstackPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, src := range map[string]string{
		"a_compatible.php":  "<?php $a = 1 + 2; echo $a;",
		"b_unsupported.php": "<?php $a = new class { public $x = 1; };",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	diags, err := lint.FlatstackPaths([]string{dir})
	if err != nil {
		t.Fatalf("FlatstackPaths: %v", err)
	}
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %+v, want one per file", diags)
	}
	if !strings.Contains(diags[0].Message, "[flatstack compatible]") {
		t.Errorf("the first file reads %q", diags[0].Message)
	}
	if !strings.Contains(diags[1].Message, "[flatstack unsupported]") {
		t.Errorf("the second file reads %q", diags[1].Message)
	}
	if diags[0].File != filepath.Join(dir, "a_compatible.php") {
		t.Errorf("the first diagnostic names %q", diags[0].File)
	}
}

func TestPathsReportsParseErrorsAndContinues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := map[string]string{
		"invalid.php": `<?php class Foo {`,
		"lint.php":    `<?php if ($value = true) {}`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	diags, err := lint.Paths([]string{dir})
	if err != nil {
		t.Fatalf("Paths returned a parser error instead of continuing: %v", err)
	}
	if len(diags) != 2 {
		t.Fatalf("got %d diagnostics, want parser and lint diagnostics: %+v", len(diags), diags)
	}
	if got := diags[0].Message; len(got) < len("parse error:") || got[:len("parse error:")] != "parse error:" {
		t.Fatalf("first diagnostic = %q, want parse error", got)
	}
	if got := diags[1].Message; got != "assignment in conditional statement" {
		t.Fatalf("second diagnostic = %q, want assignment diagnostic", got)
	}
}

func TestPathsLintsPHPSectionOfPHPTFixture(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "assignment.phpt")
	src := `name: assignment condition
description: Lint the PHP section only.
---
<?php
if ($value = true) {}
---
done
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	diags, err := lint.Paths([]string{path})
	if err != nil {
		t.Fatalf("Paths returned an error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want assignment diagnostic: %+v", len(diags), diags)
	}
	if got := diags[0].Message; got != "assignment in conditional statement" {
		t.Fatalf("diagnostic = %q, want assignment diagnostic", got)
	}
	if got := diags[0].Line; got != 5 {
		t.Fatalf("diagnostic line = %d, want physical fixture line 5", got)
	}
}

// A violated interface contract is a lint finding on the class's own line, so
// the author reads the missing method before the file runs.
func TestFileReportsInterfaceContract(t *testing.T) {
	t.Parallel()
	src := "<?php\ninterface Reader {\n\tfunction get($key);\n\tfunction has($key);\n}\n\nclass Store implements Reader {\n\tfunction get($key) {\n\t\treturn $key;\n\t}\n}\n"
	diags, err := lint.File("store.php", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want one", diags)
	}
	want := "store.php:7: class Store does not declare method has() required by interface Reader"
	if got := diags[0].String(); got != want {
		t.Errorf("diagnostic = %q, want %q", got, want)
	}
}

// A class satisfying its contract, and one naming an interface no declaration
// defines, both lint clean: the second is a built-in name such as Countable,
// which phpscript does not declare.
func TestFileAcceptsSatisfiedAndUndeclaredInterfaces(t *testing.T) {
	t.Parallel()
	src := "<?php\ninterface Reader {\n\tfunction get($key);\n}\n\nclass Store implements Reader, Countable {\n\tfunction get($key) {\n\t\treturn $key;\n\t}\n}\n"
	diags, err := lint.File("store.php", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %v, want none", diags)
	}
}

// The linter reports a JSON_* constant as a warning. Nothing defines the name,
// so it arrives as null and json_encode ignores it: the call runs and encodes,
// and what the author loses is the formatting they wrote.
func TestLintJSONFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		src   string
		lines []int
	}{
		{
			name:  "one flag",
			src:   "<?php\necho json_encode($a, JSON_PRETTY_PRINT);\n",
			lines: []int{2},
		},
		{
			name:  "several on one line",
			src:   "<?php\necho json_encode($a, JSON_HEX_TAG | JSON_HEX_AMP);\n",
			lines: []int{2, 2},
		},
		{
			name:  "no flags is clean",
			src:   "<?php\necho json_encode($a);\n",
			lines: nil,
		},
		{
			// The name inside a string is not a use, so a script probing for
			// the constant reads as written.
			name:  "a string is not a use",
			src:   "<?php\nvar_dump(defined(\"JSON_PRETTY_PRINT\"));\n$s = \"JSON_HEX_TAG\";\n",
			lines: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diags, err := lint.File("probe.php", test.src)
			if err != nil {
				t.Fatal(err)
			}
			var got []int
			for _, d := range diags {
				if !strings.Contains(d.Message, "the JSON encoding is not") {
					continue
				}
				got = append(got, d.Line)
			}
			if len(got) != len(test.lines) {
				t.Fatalf("lines = %v, want %v", got, test.lines)
			}
			for i := range got {
				if got[i] != test.lines[i] {
					t.Fatalf("lines = %v, want %v", got, test.lines)
				}
			}
		})
	}
}

// A call to a function nothing defines is the "call to undefined function"
// error waiting for the line to run; the linter reports it first. A name the
// file declares (at any nesting), a registered binding, or one the source
// guards with function_exists lints clean.
func TestFileReportsUndefinedFunctions(t *testing.T) {
	t.Parallel()
	src := `<?php
echo strlen("known binding"), "\n";
echo undefined_function("nothing registers this");
declared();
conditional();
if (function_exists("imagetypes")) {
	imagetypes();
}
function declared() { return 1; }
if (true) {
	function conditional() { return 2; }
}
$sorter = function ($rows) {
	return undefined_in_closure();
};
`
	diags, err := lint.File("funcs.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	want := []string{
		`funcs.php:3: call to undefined function undefined_function()`,
		`funcs.php:14: call to undefined function undefined_in_closure()`,
	}
	if len(diags) != len(want) {
		t.Fatalf("diagnostics = %v, want %d findings", diags, len(want))
	}
	for i, w := range want {
		if got := diags[i].String(); got != w {
			t.Errorf("diagnostic %d = %q, want %q", i, got, w)
		}
	}
}

// The check reports a class reference nothing declares (a `new`, a static
// call, property or constant) with the message the runtime would raise.
// Classes declared in the file, registered host classes, anonymous classes and
// the contextual self/static names lint clean.
func TestFileReportsUndefinedClasses(t *testing.T) {
	t.Parallel()
	src := `<?php
class Declared {
	public static function make() {
		return new self();
	}
}
$a = new Declared();
$b = new Exception("registered host class");
$c = new ReflectionClass("nope");
ReflectionMethod::export();
$anon = new class {
	function touch() {}
};
echo Missing::NAME;
echo Missing::class, "\n";
$d = Missing::$prop;
`
	diags, err := lint.File("classes.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	want := []string{
		`classes.php:9: new: undefined class "ReflectionClass"`,
		`classes.php:10: static call ReflectionMethod::export(): unknown class`,
		`classes.php:14: class constant Missing::NAME: unknown class`,
		`classes.php:16: static property Missing::$prop: unknown class`,
	}
	if len(diags) != len(want) {
		t.Fatalf("diagnostics = %v, want %d findings", diags, len(want))
	}
	for i, w := range want {
		if got := diags[i].String(); got != w {
			t.Errorf("diagnostic %d = %q, want %q", i, got, w)
		}
	}
}

// A function declared twice, or declared over a name the runtime registers, is
// the RedeclareError waiting for the runtime to hoist the file; the linter
// reports it first. A single declaration, a class method, and a name the
// source guards with function_exists all lint clean; the last is the polyfill
// idiom, where declaring over an absent built-in is what the case covers.
func TestFileReportsRedeclaredFunctions(t *testing.T) {
	t.Parallel()
	src := `<?php
function once_only() { return 1; }
function twice() { return 1; }
function twice() { return 2; }
function strlen($s) { return 0; }
if (!function_exists("str_contains")) {
	function str_contains($h, $n) { return false; }
}
class Holder {
	public function twice() { return 3; }
}
`
	diags, err := lint.File("redeclare.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	want := []string{
		`redeclare.php:4: Cannot redeclare function twice() (previously declared on line 3)`,
		`redeclare.php:5: Cannot redeclare function strlen()`,
	}
	if len(diags) != len(want) {
		t.Fatalf("diagnostics = %v, want %d findings", diags, len(want))
	}
	for i, w := range want {
		if got := diags[i].String(); got != w {
			t.Errorf("diagnostic %d = %q, want %q", i, got, w)
		}
	}
}

func TestLintTypeReassign(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		src   string
		want  []string
		lines []int
	}{
		{
			name:  "int over string warns",
			src:   "<?php\n$a = \"text\";\n$a = 1;\n",
			want:  []string{"no reassignment: $a previously declared as string"},
			lines: []int{3},
		},
		{
			name: "same type is clean",
			src:  "<?php\n$a = \"x\";\n$a = \"y\";\n",
		},
		{
			name: "computed values say nothing",
			src:  "<?php\n$a = \"x\";\n$a = strlen($a);\n",
		},
		{
			name: "null fixes no type",
			src:  "<?php\n$a = null;\n$a = 1;\n$a = \"s\";\n",
			want: []string{"no reassignment: $a previously declared as int"},
		},
		{
			name: "function scope is fresh",
			src:  "<?php\n$a = \"text\";\nfunction f() {\n\t$a = 1;\n}\n",
		},
		{
			name:  "branches share the scope",
			src:   "<?php\nif ($c) {\n\t$a = 1;\n} else {\n\t$a = \"s\";\n}\n",
			want:  []string{"no reassignment: $a previously declared as int"},
			lines: []int{5},
		},
		{
			name:  "float over int in a method",
			src:   "<?php\nclass C {\n\tfunction m() {\n\t\t$x = 1;\n\t\t$x = 1.5;\n\t}\n}\n",
			want:  []string{"no reassignment: $x previously declared as int"},
			lines: []int{5},
		},
		{
			name: "negative numbers keep their class",
			src:  "<?php\n$n = 1;\n$n = -5;\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags, err := lint.File("test.php", tc.src)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			var lines []int
			for _, d := range diags {
				if strings.Contains(d.Message, "no reassignment") {
					// Advisory by decision: the runtime retypes the way PHP
					// does, so the finding must not fail the lint run.
					if d.Fatal {
						t.Errorf("finding %q is fatal, want advisory", d.Message)
					}
					got = append(got, d.Message)
					lines = append(lines, d.Line)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("findings = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("finding[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
			for i := range tc.lines {
				if lines[i] != tc.lines[i] {
					t.Errorf("line[%d] = %d, want %d", i, lines[i], tc.lines[i])
				}
			}
		})
	}
}

func TestLintStringOffset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"read on a declared string", "<?php\n$s = \"text\";\necho $s[0];\n", 1},
		{"negative offset", "<?php\n$s = \"text\";\necho $s[-1];\n", 1},
		{"write through the offset", "<?php\n$s = \"text\";\n$s[0] = \"T\";\n", 1},
		{"array index stays silent", "<?php\n$a = array(1, 2);\necho $a[0];\n", 0},
		{"unknown variable stays silent", "<?php\necho $rows[0];\n", 0},
		{"substr is the endorsed spelling", "<?php\n$s = \"text\";\necho substr($s, 0, 1);\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags, err := lint.File("test.php", tc.src)
			if err != nil {
				t.Fatal(err)
			}
			got := 0
			for _, d := range diags {
				if strings.Contains(d.Message, "string offset") {
					got++
					if d.Fatal {
						t.Errorf("finding %q is fatal, want advisory", d.Message)
					}
				}
			}
			if got != tc.want {
				t.Fatalf("findings = %d, want %d (%v)", got, tc.want, diags)
			}
		})
	}
}
