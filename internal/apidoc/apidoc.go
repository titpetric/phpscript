// Package apidoc generates docs/reference/extensions/implemented-apis.md.
// It combines two sources of truth: the live runtime, which says what is
// registered and with which Go signature, and a scan of the Go source, which
// carries the doc comment written next to each registration and the parameter
// names the reflect API cannot see. See scripts/list-apis for the entry point.
package apidoc

import (
	"go/ast"
	"reflect"
	goruntime "runtime"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// modulePrefix maps a runtime symbol name back to a package directory in the
// scanned source tree.
const modulePrefix = "github.com/titpetric/phpscript/"

// Param is one PHP-visible parameter of a binding.
type Param struct {
	Name     string // without the dollar sign
	Type     string // PHP type name, empty for a by-reference out parameter
	Variadic bool
	ByRef    bool // the runner's by-reference setter wrapper
}

// Func is one registered function with everything the renderer needs.
type Func struct {
	Name    string
	Package string   // Go package directory relative to the module root
	Area    string   // subsection within the package, from the register* func
	Comment []string // doc comment lines without comment markers
	Params  []Param
	Returns string // PHP type name, "void" when the Go func returns nothing
}

// Method is one PHP-callable method on a registered class.
type Method struct {
	Name    string // PHP spelling, snake_case
	Comment []string
	Params  []Param
	Returns string
}

// Class is one registered class. Classes sharing a constructor (the SPL
// exception set) collapse into one entry with the extra names in Aliases.
type Class struct {
	Name    string
	Aliases []string
	Package string
	Comment []string
	Params  []Param // constructor parameters
	Methods []Method
}

type constructorGroup struct {
	names []string
	ctor  any
}

// Generate renders the implemented-apis markdown for the runtime rt, reading
// doc comments and parameter names from the Go source rooted at srcRoot.
func Generate(rt *runner.Runtime, srcRoot string) (string, error) {
	src, err := scan(srcRoot)
	if err != nil {
		return "", err
	}
	funcs, classes := build(rt, src)
	return render(funcs, classes), nil
}

// Reflect is Generate's first half without the source scan: what the runtime
// registers, with the signature reflection can recover on its own. It is what a
// shipped binary can answer, since the Go source it was built from is not there
// to read doc comments and PHP-spelled parameter names out of. Those are the
// only things missing from the result; the names, types and returns are the
// same ones the generated reference is rendered from.
func Reflect(rt *runner.Runtime) ([]Func, []Class) {
	return build(rt, emptySources())
}

// build assembles the registered surface from the runtime and whatever the
// source scan found, which may be nothing.
func build(rt *runner.Runtime, src *sources) ([]Func, []Class) {
	classTypes := registeredClassTypes(rt, src)

	internal, _ := rt.DefinedFunctions()
	funcs := make([]Func, 0, len(internal))
	for _, name := range internal {
		fn, _ := rt.LookupFunc(name)
		funcs = append(funcs, buildFunc(name, fn, src, classTypes))
	}

	return funcs, buildClasses(rt, src, classTypes)
}

// buildFunc merges the scanned source entry for name with the reflected
// signature of the registered Go function. Source provides parameter names and
// comments; reflection remains authoritative for returns because it can map a
// concrete Go type back to the PHP class registered for it.
func buildFunc(name string, fn any, src *sources, classTypes map[reflect.Type]string) Func {
	out := Func{Name: name, Returns: "void"}
	entry := src.funcs[name]
	if entry != nil {
		out.Package = entry.pkg
		out.Area = entry.area
		out.Comment = entry.comment
		out.Params = entry.params
		if entry.results != nil {
			out.Returns = returnType(entry.results)
		}
	}
	if fn == nil {
		return out
	}
	t := reflect.TypeOf(fn)
	if t == nil || t.Kind() != reflect.Func {
		return out
	}
	if entry == nil || entry.params == nil {
		out.Params = reflectParams(t)
	}
	if entry == nil || entry.results == nil || hasRegisteredReturn(t, classTypes) {
		out.Returns = reflectReturn(t, classTypes)
	}
	return out
}

// buildClasses reflects over every registered constructor, grouping class
// names that share one Go constructor into a single entry.
func buildClasses(rt *runner.Runtime, src *sources, classTypes map[reflect.Type]string) []Class {
	groups := constructorGroups(rt)
	classes := make([]Class, 0, len(groups))
	for _, g := range groups {
		sort.Strings(g.names)
		classes = append(classes, buildClass(g.names, g.ctor, src, classTypes))
	}
	sort.Slice(classes, func(i, j int) bool { return classes[i].Name < classes[j].Name })
	return classes
}

// buildClass assembles one class entry: the primary name is the one a scanned
// registration site documents, falling back to the shortest of the group.
func buildClass(names []string, ctor any, src *sources, classTypes map[reflect.Type]string) Class {
	primary := primaryClassName(names, src)

	out := Class{Name: primary}
	for _, name := range names {
		if name != primary {
			out.Aliases = append(out.Aliases, name)
		}
	}

	if entry := src.ctors[primary]; entry != nil {
		out.Package = entry.pkg
		out.Comment = entry.comment
		out.Params = entry.params
	} else if pkg, decl := ctorDecl(ctor, src); decl != nil {
		// A class registered under a computed name (the SPL exception loop)
		// has no literal site to scan; the constructor's declaration, found
		// through its runtime symbol name, carries the signature, and the
		// type declaration documents the class better than a "NewX returns"
		// constructor comment does.
		out.Package = pkg
		out.Params, _ = signature(decl.Type)
		if ti := src.typeDoc(primary); ti != nil {
			out.Comment = ti.comment
		} else {
			out.Comment = declComment(decl, primary)
		}
	} else if ti := src.typeDoc(primary); ti != nil {
		out.Package = ti.pkg
		out.Comment = ti.comment
	}

	t := reflect.TypeOf(ctor)
	if t == nil || t.Kind() != reflect.Func {
		return out
	}
	if out.Params == nil {
		out.Params = reflectParams(t)
	}
	if t.NumOut() > 0 && t.Out(0).Kind() != reflect.Interface {
		out.Methods = reflectMethods(t.Out(0), src, classTypes)
	}
	return out
}

func primaryClassName(names []string, src *sources) string {
	primary := names[0]
	for _, name := range names {
		if len(name) < len(primary) {
			primary = name
		}
	}
	for _, name := range names {
		if e := src.ctors[name]; e != nil && len(e.comment) > 0 {
			primary = name
			break
		}
	}
	return primary
}

// registeredClassTypes maps the concrete value returned by each constructor
// to the PHP class name scripts use for that value. Named integer types need
// this just as much as structs: time.Duration would otherwise render as int.
func registeredClassTypes(rt *runner.Runtime, src *sources) map[reflect.Type]string {
	classTypes := map[reflect.Type]string{}
	for _, group := range constructorGroups(rt) {
		t, ok := constructorResultType(group.ctor)
		if !ok {
			continue
		}
		sort.Strings(group.names)
		classTypes[t] = primaryClassName(group.names, src)
	}
	return classTypes
}

func constructorGroups(rt *runner.Runtime) []*constructorGroup {
	byConstructor := map[uintptr]*constructorGroup{}
	groups := []*constructorGroup{}
	for _, name := range rt.DeclaredClasses() {
		ctor, ok := rt.LookupConstructor(name)
		if !ok {
			continue
		}
		key := reflect.ValueOf(ctor).Pointer()
		group := byConstructor[key]
		if group == nil {
			group = &constructorGroup{ctor: ctor}
			byConstructor[key] = group
			groups = append(groups, group)
		}
		group.names = append(group.names, name)
	}
	return groups
}

func constructorResultType(ctor any) (reflect.Type, bool) {
	t := reflect.TypeOf(ctor)
	if t == nil || t.Kind() != reflect.Func {
		return nil, false
	}
	for i := 0; i < t.NumOut(); i++ {
		out := t.Out(i)
		if out == errorType {
			continue
		}
		if out.Kind() == reflect.Interface {
			return nil, false
		}
		return out, true
	}
	return nil, false
}

// ctorDecl resolves a constructor value to its scanned declaration through
// the symbol name the Go runtime holds for it. Function literals resolve to
// nothing: their synthesised names (Register.func1) have no declaration.
func ctorDecl(ctor any, src *sources) (string, *ast.FuncDecl) {
	fn := goruntime.FuncForPC(reflect.ValueOf(ctor).Pointer())
	if fn == nil {
		return "", nil
	}
	symbol := strings.TrimPrefix(fn.Name(), modulePrefix)
	if symbol == fn.Name() {
		return "", nil
	}
	pkg, name, ok := strings.Cut(symbol, ".")
	if !ok || strings.Contains(name, ".") {
		return "", nil
	}
	if info := src.lookupDecl(pkg, name, true); info != nil {
		return pkg, info.decl
	}
	return "", nil
}

// reflectMethods lists the PHP-callable surface of the Go type a constructor
// returns. Method dispatch is case-insensitive with underscores accepted, so
// the PHP spelling is the Go name in snake_case.
func reflectMethods(t reflect.Type, src *sources, classTypes map[reflect.Type]string) []Method {
	typeName := t.Name()
	if t.Kind() == reflect.Pointer {
		typeName = t.Elem().Name()
	}
	methods := make([]Method, 0, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if infraMethods[m.Name] {
			continue
		}
		out := Method{
			Name:    camelToSnake(m.Name),
			Params:  reflectParams(m.Func.Type())[1:], // drop the receiver
			Returns: reflectReturn(m.Func.Type(), classTypes),
		}
		if decl := src.method(typeName, m.Name); decl != nil {
			out.Comment = decl.comment
			if decl.params != nil {
				out.Params = decl.params
			}
			if decl.results != nil && !hasRegisteredReturn(m.Func.Type(), classTypes) {
				out.Returns = returnType(decl.results)
			}
		}
		methods = append(methods, out)
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].Name < methods[j].Name })
	return methods
}

// infraMethods are Go interface conventions and runtime hooks, present on a
// bound type for the host's benefit, not for a script to call.
var infraMethods = map[string]bool{
	"Error": true, "String": true, "GoString": true, "SetID": true,
	"MarshalJSON": true, "UnmarshalJSON": true,
}

// returnType folds a Go result list into one PHP type. A trailing error is
// thrown, not returned, so it never shows.
func returnType(results []string) string {
	kept := make([]string, 0, len(results))
	seen := map[string]bool{}
	for _, r := range results {
		if r == "error" || seen[r] {
			continue
		}
		seen[r] = true
		kept = append(kept, r)
	}
	switch len(kept) {
	case 0:
		return "void"
	case 1:
		return kept[0]
	}
	return strings.Join(kept, "|")
}

// camelToSnake converts a Go method name to its PHP spelling: Query to query,
// SetAttribute to set_attribute, SetID to set_id.
func camelToSnake(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			prevLower := i > 0 && runes[i-1] >= 'a' && runes[i-1] <= 'z'
			nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if i > 0 && (prevLower || nextLower) {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
