package apidoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestDocs covers the index shape a shipped binary answers out of. The index
// keys a class by the PHP name a script types, and a member by the Go import
// path and type name the class binds.
func TestDocs(t *testing.T) {
	t.Parallel()
	d := &Docs{
		Classes: map[string]*SymbolDocs{"Time": {Comment: []string{"Time constructs the current instant."}}},
		Types: map[string]*TypeDocs{
			"time.Time": {Methods: map[string]*SymbolDocs{"Format": {Comment: []string{"format renders the instant."}}}},
		},
	}
	if got := d.Classes["Time"].Comment[0]; !strings.HasPrefix(got, "Time constructs") {
		t.Errorf("class comment = %q", got)
	}
	if got := d.Types["time.Time"].Methods["Format"].Comment[0]; !strings.HasPrefix(got, "format renders") {
		t.Errorf("method comment = %q", got)
	}
}

// TestSymbolDocs covers what one declaration says: its comment and the PHP
// spelling of each parameter, which reflection has to invent.
func TestSymbolDocs(t *testing.T) {
	t.Parallel()
	s := SymbolDocs{Comment: []string{"get sends a GET request."}, Params: []string{"url"}}
	if len(s.Comment) != 1 || len(s.Params) != 1 {
		t.Errorf("SymbolDocs = %+v, want one comment line and one parameter", s)
	}
}

// TestTypeDocs covers the two member tables one bound Go type carries.
func TestTypeDocs(t *testing.T) {
	t.Parallel()
	d := TypeDocs{
		Fields:  map[string][]string{"ID": {"ID is the connection."}},
		Methods: map[string]*SymbolDocs{"Query": {Params: []string{"query", "args"}}},
	}
	if len(d.Fields) != 1 || len(d.Methods) != 1 {
		t.Errorf("TypeDocs = %+v, want one field and one method", d)
	}
}

// TestEmbedded covers the index this binary carries. The pipeline generates it
// from the Go source and the binary embeds it, because a shipped binary carries
// no source tree and the two things a scan of one recovers are a doc comment
// and the PHP spelling of a parameter name.
func TestEmbedded(t *testing.T) {
	t.Parallel()
	docs := Embedded()
	if docs == nil {
		t.Fatal("Embedded returned nil, want an index or an empty one")
	}
	if len(docs.Classes) == 0 {
		t.Fatalf("the embedded index documents no class; regenerate %s", IndexPath)
	}
	client := docs.Classes["HTTP\\Client"]
	if client == nil || len(client.Comment) == 0 {
		t.Fatalf("the embedded index documents no HTTP\\Client; regenerate %s", IndexPath)
	}
	entry := docs.Types["github.com/titpetric/phpscript/stdlib/http.Client"]
	if entry == nil {
		t.Fatalf("the embedded index documents no member of stdlib/http.Client")
	}
	if got := entry.Methods["Get"]; got == nil || strings.Join(got.Params, ",") != "url" {
		t.Errorf("Get parameters = %+v, want the declared name url", got)
	}
}

// TestScanDocs covers the generator over a tree of its own: the class comment,
// the property comment and the method comment all reach the index, and the
// parameter names come from the declaration and not from the Go types.
func TestScanDocs(t *testing.T) {
	t.Parallel()
	// The tree mirrors this package's own directory, because the scan maps a
	// directory below the module root onto the import path a reflected type
	// reports, and that path keys every field comment.
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "internal", "apidoc")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package fake

func register(rt *Runtime) {
	rt.RegisterConstructor("Holder", newScannedHolder)
}

// newScannedHolder builds a holder.
func newScannedHolder(name string) *scannedHolder { return &scannedHolder{} }

// scannedHolder is the class under test.
type scannedHolder struct {
	// ID is the name the holder opened under.
	ID string

	hidden string
}

// Query runs a statement.
func (h *scannedHolder) Query(statement string) string { return statement }
`
	if err := os.WriteFile(filepath.Join(pkgDir, "holder.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := runner.New(nil, runner.Options{})
	rt.RegisterConstructor("Holder", newScannedHolder)

	docs, err := ScanDocs(rt, dir)
	if err != nil {
		t.Fatal(err)
	}

	holder := docs.Classes["Holder"]
	if holder == nil {
		t.Fatalf("the index documents no Holder: %+v", docs.Classes)
	}
	if got, want := strings.Join(holder.Params, ","), "name"; got != want {
		t.Errorf("constructor parameters = %q, want %q", got, want)
	}

	key := "github.com/titpetric/phpscript/internal/apidoc.scannedHolder"
	entry := docs.Types[key]
	if entry == nil {
		t.Fatalf("the index documents no member of %s: %+v", key, docs.Types)
	}
	if got, want := strings.Join(entry.Fields["ID"], " "), "ID is the name the holder opened under."; got != want {
		t.Errorf("ID documents %q, want %q", got, want)
	}
	if _, ok := entry.Fields["hidden"]; ok {
		t.Error("an unexported field reached the index, and no script can read it")
	}
	if got := entry.Methods["Query"]; got == nil || strings.Join(got.Params, ",") != "statement" {
		t.Errorf("Query = %+v, want the declared parameter name statement", got)
	}
}

// TestDocs_Apply covers what the index fills in and what it leaves alone. The
// structure is reflection's answer about this build and the index carries the
// prose; a member the index does not name keeps the name reflection invented.
func TestDocs_Apply(t *testing.T) {
	t.Parallel()
	classes := []Class{{
		Name:    "Holder",
		GoType:  "fake.Holder",
		Params:  []Param{{Name: "string", Type: "string"}},
		Fields:  []Field{{Name: "id", GoName: "ID", Type: "string"}},
		Methods: []Method{{Name: "query", GoName: "Query", Params: []Param{{Name: "string", Type: "string"}}}},
	}}
	d := &Docs{
		Classes: map[string]*SymbolDocs{"Holder": {Comment: []string{"Holder holds."}, Params: []string{"name"}}},
		Types: map[string]*TypeDocs{"fake.Holder": {
			Fields:  map[string][]string{"ID": {"ID is the name."}},
			Methods: map[string]*SymbolDocs{"Query": {Comment: []string{"query runs."}, Params: []string{"statement"}}},
		}},
	}
	d.Apply(classes)

	c := classes[0]
	if got, want := strings.Join(c.Comment, " "), "Holder holds."; got != want {
		t.Errorf("class comment = %q, want %q", got, want)
	}
	if got, want := c.Signature(), "public function __construct(string $name)"; got != want {
		t.Errorf("constructor = %q, want %q", got, want)
	}
	if got, want := strings.Join(c.Fields[0].Comment, " "), "ID is the name."; got != want {
		t.Errorf("property comment = %q, want %q", got, want)
	}
	if got, want := c.Methods[0].Signature(), "public function query(string $statement): "; got != want {
		t.Errorf("method = %q, want %q", got, want)
	}
}

// TestDocsApplyKeepsReflectedNamesOnMismatch pins the guard on renaming by
// position. A signature changed without regenerating the index leaves a count
// that disagrees, and renaming then moves a name onto another argument.
func TestDocsApplyKeepsReflectedNamesOnMismatch(t *testing.T) {
	t.Parallel()
	classes := []Class{{Name: "Holder", Params: []Param{{Name: "string", Type: "string"}, {Name: "int", Type: "int"}}}}
	d := &Docs{Classes: map[string]*SymbolDocs{"Holder": {Params: []string{"name"}}}}
	d.Apply(classes)
	if got, want := classes[0].Params[0].Name, "string"; got != want {
		t.Errorf("parameter name = %q, want the reflected %q", got, want)
	}
}

// TestDocsPrune covers what the generated file holds: the source text that
// exists and no empty entry around it, and a type documenting nothing leaves no
// line in the index.
func TestDocsPrune(t *testing.T) {
	t.Parallel()
	d := &Docs{
		Classes: map[string]*SymbolDocs{"Bare": {}, "Holder": {Comment: []string{"Holder holds."}}},
		Types: map[string]*TypeDocs{
			"fake.Bare":   {Fields: map[string][]string{}, Methods: map[string]*SymbolDocs{"Noop": {}}},
			"fake.Holder": {Methods: map[string]*SymbolDocs{"Query": {Params: []string{"statement"}}}},
		},
	}
	d.prune()
	if _, ok := d.Classes["Bare"]; ok {
		t.Error("a class documenting nothing survived the prune")
	}
	if _, ok := d.Types["fake.Bare"]; ok {
		t.Error("a type documenting nothing survived the prune")
	}
	if d.Types["fake.Holder"].Fields != nil {
		t.Error("an empty field table survived the prune")
	}
	if _, ok := d.Types["fake.Holder"].Methods["Query"]; !ok {
		t.Error("a method naming its parameters was pruned")
	}
}

// scannedHolder is the class TestScanDocs registers, declared here so the
// runtime binds a real Go type while the scan reads the source above.
type scannedHolder struct {
	// ID is the name the holder opened under.
	ID string

	hidden string
}

// Query runs a statement.
func (h *scannedHolder) Query(statement string) string { return statement }

// newScannedHolder builds a holder.
func newScannedHolder(name string) *scannedHolder { return &scannedHolder{ID: name} }
