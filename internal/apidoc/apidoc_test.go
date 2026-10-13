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

// TestReflect covers what a shipped binary can answer about itself. The Go
// source it was built from is not on disk, so there are no doc comments and no
// PHP-spelled parameter names; the names, types and returns are the same ones
// the committed reference is rendered from, and the placeholder parameter
// names come from the types.
func TestReflect(t *testing.T) {
	rt := runner.New(nil, runner.Options{})
	rt.RegisterConstructor("Time", newGeneratedTime)
	rt.RegisterConstructor(`Time\Duration`, newGeneratedDuration)
	rt.RegisterFunc("DateTime::now", generatedNow)
	rt.RegisterFunc("DateTime::since", generatedSince)

	funcs, classes := Reflect(rt)

	byName := map[string]Func{}
	for _, f := range funcs {
		byName[f.Name] = f
	}
	for _, want := range []struct {
		name      string
		signature string
	}{
		{name: "DateTime::now", signature: "DateTime::now(): Time"},
		{name: "DateTime::since", signature: `DateTime::since(object $value): Time\Duration`},
	} {
		got, ok := byName[want.name]
		if !ok {
			t.Errorf("%s: not reflected", want.name)
			continue
		}
		if got.Signature() != want.signature {
			t.Errorf("%s signature = %q, want %q", want.name, got.Signature(), want.signature)
		}
		if len(got.Comment) != 0 {
			t.Errorf("%s carries comment %v, want none without the source", want.name, got.Comment)
		}
	}

	byClass := map[string]Class{}
	for _, c := range classes {
		byClass[c.Name] = c
	}
	time, ok := byClass["Time"]
	if !ok {
		t.Fatalf("Time: not reflected, classes = %+v", byClass)
	}
	if got, want := time.Signature(), "public function __construct()"; got != want {
		t.Errorf("Time constructor = %q, want %q", got, want)
	}

	methods := map[string]string{}
	for _, m := range time.Methods {
		methods[m.Name] = m.Signature()
	}
	for name, want := range map[string]string{
		"add": "public function add(int $num): Time",
		// The receiver is reflected as the first parameter and is dropped
		// after the names are deduped, so the one that is kept is $value2.
		"sub":      `public function sub(object $value2): Time\Duration`,
		"location": "public function location(): object",
	} {
		if methods[name] != want {
			t.Errorf("Time::%s = %q, want %q", name, methods[name], want)
		}
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
