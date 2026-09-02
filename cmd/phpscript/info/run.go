package info

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/titpetric/cli"

	"github.com/titpetric/phpscript/internal/flags"
	phplist "github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

const Name = "Print runtime environment"

type Options struct {
	Verbose bool
}

// NewCommand creates a new info command. The verbosity dial is the shared -v:
// here it lists every bound function, class and method under the summary.
func NewCommand(globals *flags.Options) *cli.Command {
	return &cli.Command{
		Name:  "info",
		Title: Name,
		Run: func(ctx context.Context, args []string) error {
			return Run(ctx, args, Options{Verbose: globals.Verbose})
		},
	}
}

func Run(ctx context.Context, args []string, opts Options) error {
	if len(args) > 0 {
		return printSourceTree(args)
	}
	rt := runner.New(os.Stdout, runner.Options{SAPI: "cli", RootFS: os.DirFS(".")})
	stdlib.Register(rt)
	stdlib.RegisterFS(rt, ".")
	// The request-aware functions (header, http_response_code, getallheaders)
	// are installed per request rather than by stdlib.Register. Without this
	// the counts here are short of what `phpscript list --stdlib` and the
	// generated reference report, and the three disagree about the same
	// runtime.
	runner.NewContext().Register(rt)
	fmt.Fprintln(os.Stdout, "# phpscript")
	fmt.Fprintln(os.Stdout)
	if err := rt.PHPInfo(); err != nil {
		return err
	}
	if !opts.Verbose {
		return nil
	}
	printBindings(rt)
	return nil
}

func printBindings(rt *runner.Runtime) {
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "# Classes")
	for _, name := range rt.DeclaredClasses() {
		fmt.Fprintf(os.Stdout, "\n## %s\n\n", name)
		if ctor, ok := rt.LookupConstructor(name); ok {
			printHostClass(name, ctor)
			continue
		}
		fmt.Fprintf(os.Stdout, "Usage:\n\n- `new %s()`\n", name)
	}
	internal, _ := rt.DefinedFunctions()
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "## Functions")
	fmt.Fprintln(os.Stdout)
	for _, name := range internal {
		fmt.Fprintf(os.Stdout, "- `%s()`\n", name)
	}
}

func printHostClass(name string, ctor any) {
	t := reflect.TypeOf(ctor)
	if t.Kind() != reflect.Func {
		fmt.Fprintf(os.Stdout, "Usage:\n\n- `new %s()`\n", name)
		return
	}
	fmt.Fprintln(os.Stdout, "Usage:")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "- `new %s(%s)`\n", name, ctorParams(t))
	ret := ctorReturn(t)
	if ret == nil {
		return
	}
	for i := 0; i < ret.NumMethod(); i++ {
		m := ret.Method(i)
		if !m.IsExported() {
			continue
		}
		fmt.Fprintf(os.Stdout, "- `func (*%s) %s(%s)`\n", shortType(ret), phpMethod(m.Name), methodParams(m.Type))
	}
}

func ctorParams(t reflect.Type) string {
	var parts []string
	for i := 0; i < t.NumIn(); i++ {
		in := t.In(i)
		if i == 0 && in == reflect.TypeOf((*context.Context)(nil)).Elem() {
			continue
		}
		parts = append(parts, "$"+paramName(in, i))
	}
	return strings.Join(parts, ", ")
}

func ctorReturn(t reflect.Type) reflect.Type {
	for i := 0; i < t.NumOut(); i++ {
		out := t.Out(i)
		if out.Kind() == reflect.Interface && out.Name() == "error" {
			continue
		}
		for out.Kind() == reflect.Pointer {
			out = out.Elem()
		}
		return out
	}
	return nil
}

func methodParams(t reflect.Type) string {
	var parts []string
	start := 0
	if t.NumIn() > 0 && t.In(0).Kind() == reflect.Pointer {
		start = 1
	}
	for i := start; i < t.NumIn(); i++ {
		in := t.In(i)
		if in == reflect.TypeOf((*context.Context)(nil)).Elem() {
			continue
		}
		parts = append(parts, "$"+paramName(in, i))
	}
	return strings.Join(parts, ", ")
}

func paramName(t reflect.Type, i int) string {
	if t.Kind() == reflect.String {
		return "name"
	}
	return fmt.Sprintf("arg%d", i)
}

func phpMethod(name string) string {
	if name == "" {
		return name
	}
	return strings.ToLower(name[:1]) + name[1:]
}

func shortType(t reflect.Type) string {
	n := t.Name()
	if n == "" {
		return t.String()
	}
	return n
}

func printSourceTree(paths []string) error {
	files, err := phplist.ExpandFiles(paths)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "# Source")
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		prog, err := parser.Parse(string(src))
		if err != nil {
			continue
		}
		var classes []*model.ClassDecl
		var funcs []*model.FuncDecl
		for _, s := range prog.Stmts {
			switch n := s.(type) {
			case *model.ClassDecl:
				classes = append(classes, n)
			case *model.FuncDecl:
				funcs = append(funcs, n)
			}
		}
		if len(classes) == 0 && len(funcs) == 0 {
			continue
		}
		fmt.Fprintf(os.Stdout, "\n## %s\n", file)
		for _, c := range classes {
			fmt.Fprintf(os.Stdout, "\n### %s\n\nUsage:\n\n- `new %s()`\n", c.Name, c.Name)
			for _, m := range c.Methods {
				fmt.Fprintf(os.Stdout, "- `function %s(%s)`\n", m.Name, paramList(m.Params))
			}
		}
		for _, f := range funcs {
			fmt.Fprintf(os.Stdout, "\n- `function %s(%s)`\n", f.Name, paramList(f.Params))
		}
	}
	return nil
}

func paramList(params []model.Param) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = "$" + p.Name
	}
	return strings.Join(parts, ", ")
}
