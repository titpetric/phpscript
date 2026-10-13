package apidoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAreaName(t *testing.T) {
	cases := map[string]string{
		"registerStrings": "strings",
		"registerJSON":    "json",
		"Register":        "",
		"RegisterFS":      "",
		"init":            "",
	}
	for in, want := range cases {
		if got := areaName(in); got != want {
			t.Errorf("areaName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestScan exercises the source scan over a synthetic registration file: the
// registration-site comment, the godoc fallback with its leading symbol
// rewritten, signature extraction, and the by-reference setter convention.
func TestScan(t *testing.T) {
	dir := t.TempDir()
	src := `package fake

// registerFake installs the fake area.
func registerFake(rt *Runtime) {
	// upper returns $string uppercased.
	rt.RegisterFunc("upper", func(s string) string { return s })
	rt.RegisterFunc("scan", func(pattern, subject string, matches ...func(any)) int64 { return 0 })
	rt.RegisterFunc("named", namedShim)
	rt.RegisterConstructor("Fake\\Thing", NewThing)
}

// namedShim reports nothing in particular.
func namedShim(a any) bool { return false }

// NewThing constructs the thing a script holds.
func NewThing(name string) *Thing { return nil }

// Thing is a fake bound class.
type Thing struct{}
`
	if err := os.WriteFile(filepath.Join(dir, "fake.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	scanned, err := scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	upper := scanned.funcs["upper"]
	if upper == nil {
		t.Fatal("upper: not scanned")
	}
	if got := strings.Join(upper.comment, "\n"); got != "upper returns $string uppercased." {
		t.Errorf("upper comment = %q", got)
	}
	// The area is read off the installer's name with "register" taken off and
	// the rest spelled in words, so registerFake installs the fake area.
	if upper.area != "fake" {
		t.Errorf("upper area = %q, want fake", upper.area)
	}

	scanFn := scanned.funcs["scan"]
	if scanFn == nil || len(scanFn.params) != 3 {
		t.Fatalf("scan params = %+v", scanFn)
	}
	if p := scanFn.params[2]; !p.ByRef || !p.Variadic || p.Name != "matches" {
		t.Errorf("scan matches param = %+v", p)
	}

	named := scanned.funcs["named"]
	if named == nil {
		t.Fatal("named: not scanned")
	}
	if got := strings.Join(named.comment, "\n"); got != "named reports nothing in particular." {
		t.Errorf("named comment = %q, want the godoc with the symbol rewritten", got)
	}

	ctor := scanned.ctors[`Fake\Thing`]
	if ctor == nil {
		t.Fatal(`Fake\Thing: not scanned`)
	}
	if len(ctor.params) != 1 || ctor.params[0].Type != "string" {
		t.Errorf(`Fake\Thing params = %+v`, ctor.params)
	}
}

// TestScanQualifiedRegistration covers the three shapes a selector takes as the
// registered expression: a package in the tree, a method on a value, and a
// package outside the tree. The last one has no declaration to read, and
// borrowing one that shares the name published the wrong parameters.
func TestScanQualifiedRegistration(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"engine/compile.go": `package engine

// Compile lowers an ast.
func Compile(ast *Program) (*Program, error) { return nil, nil }
`,
		"host/host.go": `package host

// SetRoot records the root.
func (rt *Runtime) SetRoot(dir string) bool { return false }
`,
		"area/area.go": `package area

// register installs the area.
func register(rt *Runtime) {
	rt.RegisterConstructor("Area\\Compile", stdregexp.Compile)
	rt.RegisterFunc("in_tree", engine.Compile)
	rt.RegisterFunc("chroot", rt.SetRoot)
}
`,
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	scanned, err := scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	// A package in the tree resolves, parameter name and all.
	inTree := scanned.funcs["in_tree"]
	if inTree == nil || len(inTree.params) != 1 || inTree.params[0].Name != "ast" {
		t.Errorf("in_tree params = %+v, want one named ast", inTree)
	}

	// A method reached through a value resolves, the qualifier being a variable.
	chroot := scanned.funcs["chroot"]
	if chroot == nil || len(chroot.params) != 1 || chroot.params[0].Name != "dir" {
		t.Errorf("chroot params = %+v, want one named dir", chroot)
	}

	// A package outside the tree has no signature here, whatever names it
	// shares with one inside it.
	ctor := scanned.ctors[`Area\Compile`]
	if ctor == nil {
		t.Fatal(`Area\Compile: not scanned`)
	}
	if ctor.params != nil {
		t.Errorf(`Area\Compile params = %+v, want none so reflection decides`, ctor.params)
	}
}

// TestScanReadsNoRegistrations covers the two empty results the generator has
// to survive: a tree with no registration in it, and the empty set Reflect
// passes when there is no source to read at all. Both leave every lookup
// answering nil, so build falls back to reflection for each entry.
func TestScanReadsNoRegistrations(t *testing.T) {
	dir := t.TempDir()
	src := `package quiet

// Helper computes nothing anyone registered.
func Helper(name string) string { return name }
`
	if err := os.WriteFile(filepath.Join(dir, "quiet.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	scanned, err := scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, src := range []*sources{scanned, emptySources()} {
		if len(src.funcs) != 0 {
			t.Errorf("funcs = %+v, want none", src.funcs)
		}
		if len(src.ctors) != 0 {
			t.Errorf("ctors = %+v, want none", src.ctors)
		}
		if src.funcs["upper"] != nil || src.ctors["Thing"] != nil {
			t.Error("a lookup against an empty scan answered a value")
		}
	}
}
