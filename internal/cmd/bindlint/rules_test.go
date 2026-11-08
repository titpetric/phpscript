package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// parseSource builds a one-file package out of src, the way parseDir builds
// one out of a directory.
func parseSource(t *testing.T, src string) *pkg {
	t.Helper()
	p := &pkg{
		dir:     ".",
		root:    ".",
		fset:    token.NewFileSet(),
		funcs:   map[string][]*ast.FuncDecl{},
		methods: map[string][]*ast.FuncDecl{},
	}
	file, err := parser.ParseFile(p.fset, "bind.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p.files = []*ast.File{file}
	p.index()
	return p
}

// TestRules covers one snippet per rule and per reason a narrowing has to
// exist. Each case is the whole of a package: the registration, the binding
// and whatever helper it calls.
func TestRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
		want string // the finding's message, "" for no finding
	}{
		{
			name: "parameter of the runtime type",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(a *model.Array) int64 { return int64(a.Len()) })
}`,
			want: "parameter $a is *model.Array",
		},
		{
			name: "variadic parameter of the runtime type",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(rows ...*model.Array) int64 { return int64(len(rows)) })
}`,
			want: "parameter $rows is ...*model.Array",
		},
		{
			name: "narrowing the binding only reads",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(v any) int64 {
		a, ok := v.(*model.Array)
		if !ok {
			return 0
		}
		return int64(a.Len())
	})
}`,
			want: "narrows a to *model.Array",
		},
		{
			name: "narrowing an object the binding only reads",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(v any) int64 {
		o, ok := v.(*model.Object)
		if !ok {
			return 0
		}
		return int64(o.Len())
	})
}`,
			want: "narrows o to *model.Object",
		},
		{
			name: "the collection helpers read every shape",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(v any) int64 {
		if a, ok := v.(*model.Array); ok {
			return int64(a.Len())
		}
		n, _ := modelutil.LenValues(v)
		return int64(n)
	})
}`,
		},
		{
			name: "the mutator refuses the rest with an error",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(v any) (int64, error) {
		a, ok := v.(*model.Array)
		if !ok {
			return 0, fmt.Errorf("f: expected an array, got %T", v)
		}
		return int64(a.Len()), nil
	})
}`,
		},
		{
			name: "a capability a Go-native value has not got",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(v any, value any) int64 {
		a, ok := v.(*model.Array)
		if !ok {
			return 0
		}
		a.Append(value)
		return int64(a.Len())
	})
}`,
		},
		{
			name: "a type switch with a case for the native shape",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(v any) int64 {
		switch x := v.(type) {
		case *model.Array:
			return int64(x.Len())
		case []any:
			return int64(len(x))
		}
		return 0
	})
}`,
		},
		{
			name: "a type switch with no case for the native shape",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(v any) int64 {
		switch x := v.(type) {
		case *model.Array:
			return int64(x.Len())
		case string:
			return int64(len(x))
		}
		return 0
	})
}`,
			want: "narrows x to *model.Array",
		},
		{
			name: "a narrowing in a function no binding calls",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(v any) int64 { return 0 })
}

func unregistered(v any) int64 {
	a, ok := v.(*model.Array)
	if !ok {
		return 0
	}
	return int64(a.Len())
}`,
		},
		{
			name: "a method of the constructed class",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterConstructor("Box", NewBox)
}

func NewBox() *Box { return &Box{} }

type Box struct{}

func (b *Box) Fill(items *model.Array) {}`,
			want: "Box::Fill: parameter $items is *model.Array",
		},
		{
			name: "a method value",
			src: `package b
func Register(rt *runner.Runtime) {
	r := &reader{}
	rt.RegisterFunc("f", r.read)
}

type reader struct{}

func (r *reader) read(pairs *model.Array) int64 { return int64(pairs.Len()) }`,
			want: "parameter $pairs is *model.Array",
		},
		{
			name: "the literal a factory returns",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", counter(" "))
}

func counter(cutset string) func(*model.Array) int64 {
	return func(pairs *model.Array) int64 { return int64(pairs.Len()) }
}`,
			want: "parameter $pairs is *model.Array",
		},
		{
			name: "a parameter of the interface only a runtime value satisfies",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(values model.Collection) int64 { return int64(values.Len()) })
}`,
			want: "parameter $values is model.Collection",
		},
		{
			name: "a parameter of the Go-native slice",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(names []string) int64 { return int64(len(names)) })
}`,
			want: "parameter $names is []string",
		},
		{
			name: "a parameter of the Go-native map",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(options map[string]any) int64 { return int64(len(options)) })
}`,
			want: "parameter $options is map[string]any",
		},
		{
			name: "a parameter of bytes is a php string",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(data []byte) int64 { return int64(len(data)) })
}`,
			want: "",
		},
		{
			name: "a variadic parameter takes one value per argument",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(parts ...string) int64 { return int64(len(parts)) })
}`,
			want: "",
		},
		{
			name: "a type switch whose only other case is a php string",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(v any) int64 {
		switch x := v.(type) {
		case *model.Array:
			return int64(x.Len())
		case []byte:
			return int64(len(x))
		}
		return 0
	})
}`,
			want: "narrows x to *model.Array",
		},
		{
			name: "a reader of another argument excuses nothing",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(subject any, pairs any) int64 {
		n, _ := modelutil.LenValues(subject)
		a, ok := pairs.(*model.Array)
		if !ok {
			return int64(n)
		}
		return int64(a.Len())
	})
}`,
			want: "narrows a to *model.Array",
		},
		{
			name: "an error built for another argument excuses nothing",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(name string, options any) (int64, error) {
		if name == "" {
			return 0, fmt.Errorf("f: the name is empty")
		}
		a, ok := options.(*model.Array)
		if !ok {
			return 0, nil
		}
		return int64(a.Len()), nil
	})
}`,
			want: "narrows a to *model.Array",
		},
		{
			name: "an unexported method is no binding",
			src: `package b
func Register(rt *runner.Runtime) {
	rt.RegisterConstructor("Box", NewBox)
}

func NewBox() *Box { return &Box{} }

type Box struct{}

func (b *Box) fill(items *model.Array) {}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			findings := parseSource(t, tc.src).check()
			if tc.want == "" {
				if len(findings) != 0 {
					t.Fatalf("got %d findings, want none:\n%s", len(findings), join(findings))
				}
				return
			}
			if len(findings) != 1 {
				t.Fatalf("got %d findings, want 1:\n%s", len(findings), join(findings))
			}
			if got := findings[0].String(); !strings.Contains(got, tc.want) {
				t.Errorf("the finding is %q, want it to name %q", got, tc.want)
			}
		})
	}
}

// TestReachable covers the call graph the narrowing rule reads: a binding's
// helper, the helper's helper, and a recursive call that must not loop.
func TestReachable(t *testing.T) {
	t.Parallel()
	p := parseSource(t, `package b
func Register(rt *runner.Runtime) {
	rt.RegisterFunc("f", func(v any) int64 { return first(v) })
}

func first(v any) int64 { return second(v) + first(v) }

func second(v any) int64 { return 0 }

func elsewhere(v any) int64 { return 0 }`)

	bindings := p.bindings()
	if len(bindings) != 1 {
		t.Fatalf("got %d bindings, want 1", len(bindings))
	}
	var names []string
	for _, decl := range p.reachable(bindings[0].body) {
		names = append(names, decl.Name.Name)
	}
	if got, want := strings.Join(names, ","), "first,second"; got != want {
		t.Errorf("reachable is %q, want %q", got, want)
	}
}
