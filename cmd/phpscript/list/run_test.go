package list

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/cmd/phpscript/helpdocs"
	"github.com/titpetric/phpscript/internal/apidoc"
)

// TestRunListsPaths covers the default behaviour: a markdown table of the PHP
// files a path selects.
func TestRunListsPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := "<?php\n// @route GET /hello\nclass Greeter {}\n"
	if err := os.WriteFile(filepath.Join(dir, "hello.php"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := run(&out, []string{dir}, false, false, false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"Route", "Filename", "Classes", "GET /hello", "hello.php", "Greeter"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, out.String())
		}
	}
}

// TestRunListsBindings covers --bindings: the registered surface of this
// build, with no tree involved.
func TestRunListsBindings(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := run(&out, nil, true, false, false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"| Kind", "| function |", "| class    |", "| constant |", "fnmatch", "glob"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q", want)
		}
	}
}

// TestRunListsClasses covers --classes: the registered classes as PHP class
// definitions, the shape `go doc` prints a Go type in. The arguments are class
// names and not paths, and asking about one answers about one.
func TestRunListsClasses(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := run(&out, []string{`HTTP\Client`}, false, true, false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		`class HTTP\Client`,
		"[net/http.Client](https://pkg.go.dev/net/http#Client)",
		"public function get(string $url): object;",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "class Database") {
		t.Error("a class nobody asked about is listed")
	}
}

// TestRunListsClassesAsJSON covers --classes --json: the class it renders the
// definition from, for a generator that links each member in place of a block
// of PHP. Both spellings of every symbol are in it, because such a generator
// cannot derive them alone: the Go field name behind a property, the Go method
// name behind a method, and the Go interface behind the PHP name a script
// types in a catch clause.
func TestRunListsClassesAsJSON(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := run(&out, []string{`HTTP\Mux`, "Exception"}, false, true, false, true); err != nil {
		t.Fatalf("run: %v", err)
	}

	var classes []apidoc.Class
	if err := json.Unmarshal(out.Bytes(), &classes); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	if len(classes) != 2 {
		t.Fatalf("decoded %d classes, want 2", len(classes))
	}

	mux, exception := classes[0], classes[1]
	if mux.Name != `HTTP\Mux` || exception.Name != "Exception" {
		t.Fatalf("names = %q and %q, want the two asked about", mux.Name, exception.Name)
	}
	if want := "github.com/titpetric/phpscript/stdlib/ps/http"; mux.Binds != want {
		t.Errorf("Binds = %q, want %q", mux.Binds, want)
	}
	if want := "net/http.ServeMux"; mux.Uses.Symbol() != want {
		t.Errorf("Uses = %q, want %q", mux.Uses.Symbol(), want)
	}
	if len(mux.Implements) != 1 || mux.Implements[0].Symbol() != "net/http.Handler" {
		t.Errorf("Implements = %v, want net/http.Handler", mux.Implements)
	}
	if len(mux.Methods) == 0 {
		t.Fatal("Methods is empty, want the bound type's methods")
	}
	for _, m := range mux.Methods {
		if m.GoName == "" {
			t.Errorf("method %q carries no Go name", m.Name)
		}
	}
	// Throwable is the one interface of this module a script names. The citation
	// holds both spellings: the PHP one for a catch clause, the Go one for the
	// contract the binding implements.
	if len(exception.Implements) != 1 {
		t.Fatalf("Exception implements %d interfaces, want Throwable", len(exception.Implements))
	}
	if got, want := exception.Implements[0].PHP, "Throwable"; got != want {
		t.Errorf("PHP spelling = %q, want %q", got, want)
	}
	if got, want := exception.Implements[0].Symbol(), "github.com/titpetric/phpscript/runner.Throwable"; got != want {
		t.Errorf("Go symbol = %q, want %q", got, want)
	}
}

// TestRunRejectsClassesWithAPI pins the refusal. --classes answers about the
// registered classes of this build and --api lists the methods of a scanned
// tree, and a caller that named both named one of them by mistake.
func TestRunRejectsClassesWithAPI(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	err := run(&out, nil, false, true, true, false)
	if err == nil {
		t.Fatal("run returned nil error for --classes with --api")
	}
	if want := "list --classes takes no --api"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
	if out.Len() != 0 {
		t.Errorf("wrote %q, want nothing", out.String())
	}
}

// TestNewCommand covers the one flag this command owns. --bindings switches
// the subject from a tree to the binary and it defaults off: a bare `phpscript
// list` scans paths. The constructor takes the configuration and the global
// options because one signature covers all nine commands. This command reads
// neither; both arguments are nil here.
func TestNewCommand(t *testing.T) {
	t.Parallel()
	command := NewCommand(nil, nil)

	if command.Name != "list" {
		t.Errorf("Name = %q, want %q", command.Name, "list")
	}
	if command.Title != Name {
		t.Errorf("Title = %q, want %q", command.Title, Name)
	}
	if command.Bind == nil {
		t.Fatal("Bind is nil, want the --bindings flag")
	}

	set := helpdocs.NewFlagSet("list")
	command.Bind(set)
	flag := set.Lookup("bindings")
	if flag == nil {
		t.Fatal("--bindings is not bound")
	}
	if flag.DefValue != "false" {
		t.Errorf("--bindings default = %q, want false: a bare list scans paths", flag.DefValue)
	}
}

// TestRun covers the exported entry point: run() pointed at stdout.
// The markdown table is the same one a redirected run writes and the one
// assertion worth making here is that os.Stdout is where it lands.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	source := "<?php\n// @route GET /hello\nclass Greeter {}\n"
	if err := os.WriteFile(filepath.Join(dir, "hello.php"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	saved := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	runErr := Run([]string{dir}, false, false, false, false)
	writer.Close()
	os.Stdout = saved
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	var out bytes.Buffer
	if _, err := io.Copy(&out, reader); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GET /hello", "hello.php", "Greeter"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout does not contain %q:\n%s", want, out.String())
		}
	}
}

// TestRunRejectsBindingsWithPaths pins the refusal. The two modes answer about
// different things and a caller that named both named one of them by mistake.
// The error names which one it drops, never leaving them to guess.
func TestRunRejectsBindingsWithPaths(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	err := run(&out, []string{"."}, true, false, false, false)
	if err == nil {
		t.Fatal("run returned nil error for --bindings with a path argument")
	}
	if want := "list --bindings takes no path arguments"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
	if out.Len() != 0 {
		t.Errorf("wrote %q, want nothing", out.String())
	}
}

// TestRunAPI covers --api: one table row per method, read from the parser.
func TestRunAPI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := "<?php\nnamespace App;\nclass Greeter {\n\t/** Says hello. */\n\tfunction hello(string $name): string {\n\t\treturn $name;\n\t}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "hello.php"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := run(&out, []string{dir}, false, false, true, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"| Class", "| Member", "| Signature", "| Doc", `App\Greeter`, "hello", "hello(string $name): string", "Says hello."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, out.String())
		}
	}
}

// TestRunAPIJSON covers --api --json: the rows a template consumes.
func TestRunAPIJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := "<?php\nnamespace App;\nclass Greeter {\n\tfunction hello(): string {\n\t\treturn \"hi\";\n\t}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "hello.php"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := run(&out, []string{dir}, false, false, true, true); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{`"class": "App\\Greeter"`, `"member": "hello"`, `"signature": "hello(): string"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, out.String())
		}
	}
}

// TestRunAreas covers --areas: the table names the areas, a namespace argument
// narrows it, JSON writes the rows. Another listing beside it draws an error.
func TestRunAreas(t *testing.T) {
	var out strings.Builder
	if err := runAreas(&out, []string{`Runtime\PS\Core`}, false, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `Runtime\PS\Core\`) || strings.Contains(out.String(), `Runtime\PS\Files`) {
		t.Errorf("a Runtime\\PS\\Core filter listed the wrong areas:\n%.300s", out.String())
	}
	out.Reset()
	if err := runAreas(&out, nil, false, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"layer": "ps"`) {
		t.Errorf("JSON misses the layer:\n%.300s", out.String())
	}
	if err := runAreas(&out, nil, true, false); err == nil {
		t.Error("--areas mixed with --bindings was accepted")
	}
}
