package apidoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

type generatedTime struct{}

type generatedDuration int64

type generatedLocation struct{}

type generatedUnregistered struct{}

func (generatedTime) Add(generatedDuration) generatedTime { return generatedTime{} }

func (generatedTime) Sub(generatedTime) generatedDuration { return 0 }

func (generatedTime) Location() *generatedLocation { return &generatedLocation{} }

func newGeneratedTime() generatedTime { return generatedTime{} }

func newGeneratedDuration() generatedDuration { return 0 }

func newGeneratedLocation() *generatedLocation { return &generatedLocation{} }

func generatedNow() generatedTime { return generatedTime{} }

func generatedSince(generatedTime) generatedDuration { return 0 }

func generatedLoad(string) *generatedLocation { return &generatedLocation{} }

func generatedOpaque() generatedUnregistered { return generatedUnregistered{} }

func TestCamelToSnake(t *testing.T) {
	cases := map[string]string{
		"Query":         "query",
		"SetAttribute":  "set_attribute",
		"SetID":         "set_id",
		"GetAllHeaders": "get_all_headers",
		"Incr":          "incr",
		"realUsage":     "real_usage",
	}
	for in, want := range cases {
		if got := camelToSnake(in); got != want {
			t.Errorf("camelToSnake(%q) = %q, want %q", in, got, want)
		}
	}
}

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

func TestReturnType(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, "void"},
		{[]string{"error"}, "void"},
		{[]string{"string", "error"}, "string"},
		{[]string{"int", "bool"}, "int|bool"},
		{[]string{"Time", "Time"}, "Time"},
	}
	for _, c := range cases {
		if got := returnType(c.in); got != c.want {
			t.Errorf("returnType(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderParam(t *testing.T) {
	cases := []struct {
		in   Param
		want string
	}{
		{Param{Name: "subject", Type: "string"}, "string $subject"},
		{Param{Name: "args", Type: "mixed", Variadic: true}, "mixed ...$args"},
		{Param{Name: "matches", ByRef: true}, "&$matches"},
		{Param{Name: "matches", ByRef: true, Variadic: true}, "&$matches = null"},
	}
	for _, c := range cases {
		if got := renderParam(c.in); got != c.want {
			t.Errorf("renderParam(%+v) = %q, want %q", c.in, got, c.want)
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

// Registered constructor return types are the source of truth for the PHP
// class names used by function and method return hints. This matters for named
// scalar types such as time.Duration as well as structs and pointers.
func TestGenerateResolvesRegisteredClassReturnTypes(t *testing.T) {
	dir := t.TempDir()
	source := `package fake

import stdtime "time"

func registerTime(rt *Runtime) {
	rt.RegisterFunc("DateTime::now", generatedNow)
	rt.RegisterFunc("opaque", generatedOpaque)
}

func generatedNow() stdtime.Time { return stdtime.Time{} }
func generatedOpaque() any { return nil }

type generatedTime struct{}

func (generatedTime) Add(duration int64) stdtime.Time { return stdtime.Time{} }
`
	if err := os.WriteFile(filepath.Join(dir, "time.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := runner.New(nil, runner.Options{})
	rt.RegisterConstructor("Time", newGeneratedTime)
	rt.RegisterConstructor(`Time\Duration`, newGeneratedDuration)
	rt.RegisterConstructor(`Time\Location`, newGeneratedLocation)
	rt.RegisterFunc("DateTime::now", generatedNow)
	rt.RegisterFunc("DateTime::since", generatedSince)
	rt.RegisterFunc(`Time\Location::load`, generatedLoad)
	rt.RegisterFunc("opaque", generatedOpaque)

	doc, err := Generate(rt, dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"DateTime::now(): Time",
		`DateTime::since(object $value): Time\Duration`,
		`Time\Location::load(string $string): Time\Location`,
		"public function add(int $duration): Time {}",
		`public function sub(object $value2): Time\Duration {}`,
		`public function location(): Time\Location {}`,
		"function opaque(): mixed",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("generated documentation does not contain %q:\n%s", want, doc)
		}
	}
}
