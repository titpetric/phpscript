package runner

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// Lookup resolves symName to a PHP function and returns it as T: at most one
// value and an optional trailing error. The result serves rt's goroutine alone.
func Lookup[T any](rt *Runtime, symName string) (T, error) {
	var zero T
	fn, err := rt.lookup(symName, reflect.TypeOf((*T)(nil)).Elem())
	if err != nil {
		return zero, err
	}
	return fn.Interface().(T), nil
}

// LookupError reports a symbol that resolved to nothing, to more than one
// declaration, or to a signature the requested Go type cannot express.
type LookupError struct {
	// Symbol is the name the host passed, as it spelled it.
	Symbol string

	// Candidates holds the matches when the name matched too much. Any
	// other reason leaves it empty, which separates "there is no such
	// function" from "say which one".
	Candidates []string

	// Reason says what went wrong in the words the host needs.
	Reason string
}

// Error names the symbol and the reason. It lists the matches when the name
// matched too much; a host then reads which spelling to use.
func (e *LookupError) Error() string {
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("lookup %s: %s", e.Symbol, e.Reason)
	}
	return fmt.Sprintf("lookup %s: %s: %s", e.Symbol, e.Reason, strings.Join(e.Candidates, ", "))
}

// lookup is the type-erased half of Lookup, one body shared by all instantiations.
func (rt *Runtime) lookup(symName string, typ reflect.Type) (reflect.Value, error) {
	if typ == nil || typ.Kind() != reflect.Func {
		return reflect.Value{}, &LookupError{Symbol: symName, Reason: "T is not a function type"}
	}
	shape, err := signatureResults(typ)
	if err != nil {
		return reflect.Value{}, &LookupError{Symbol: symName, Reason: err.Error()}
	}

	name, err := rt.resolveSymbol(symName)
	if err != nil {
		return reflect.Value{}, err
	}
	entry, ok := rt.lookupEntry(name)
	if !ok {
		// Resolution found the declaration and hoisting installed it. A miss
		// here means the runtime disagrees with itself.
		return reflect.Value{}, &LookupError{Symbol: symName, Reason: "resolved to " + name + ", which is not in the function table"}
	}

	return reflect.MakeFunc(typ, func(in []reflect.Value) []reflect.Value {
		value, callErr := rt.invokeEntry(entry, lookupArgs(typ, in), nil)
		return rt.lookupReturn(typ, shape, value, callErr)
	}), nil
}

// symbolSource names the file a declaration reads from.
type symbolSource struct {
	path    string
	program *model.Program
}

// resolveSymbol returns the table name for a host spelling, hoisting a tree-only declaration.
func (rt *Runtime) resolveSymbol(symName string) (string, error) {
	name := strings.TrimPrefix(strings.TrimSpace(symName), "\\")
	if name == "" {
		return "", &LookupError{Symbol: symName, Reason: "the symbol name is empty"}
	}

	table := rt.symbolIndex()
	folded := strings.ToLower(name)

	matches := table.exact[folded]
	if len(matches) == 0 {
		if declared, ok := rt.declaredUserFunc(name); ok {
			return declared, nil
		}
		matches = table.suffix[folded]
	}
	if len(matches) == 0 {
		matches = matchSymbols(slices.Collect(maps.Keys(rt.userFns)), name)
	}

	switch len(matches) {
	case 0:
		return "", &LookupError{Symbol: symName, Reason: "no PHP function of that name is declared"}
	case 1:
	default:
		slices.Sort(matches)
		return "", &LookupError{Symbol: symName, Candidates: matches, Reason: "the name matches more than one function"}
	}

	found := matches[0]
	if _, declared := rt.userFns[found]; declared {
		return found, nil
	}

	sources := table.sources[found]
	if len(sources) == 0 {
		return "", &LookupError{Symbol: symName, Reason: "resolved to " + found + ", which the tree does not declare"}
	}
	if len(sources) > 1 {
		// One qualified name declared by two files. Hoisting either would raise
		// PHP's redeclaration error the moment the other arrives and the error
		// names which files disagree instead.
		paths := make([]string, 0, len(sources))
		for _, source := range sources {
			paths = append(paths, source.path)
		}
		slices.Sort(paths)
		return "", &LookupError{Symbol: symName, Candidates: paths, Reason: found + " is declared in more than one file"}
	}
	if err := rt.hoistOnce(sources[0].program, sources[0].path); err != nil {
		return "", err
	}
	// The file counts as included from here. Its declarations are live; an
	// application that require_once's it must not declare them a second time.
	// Without this marker the owning application could not load the tree a
	// handler binds out of.
	//
	// The cost is the file's top-level code, which a later require_once now
	// skips. A file that declares handlers and also works at include time
	// takes its require before the lookup, never after.
	rt.markIncluded(sources[0].path)
	return found, nil
}

// symbolTable indexes the tree's free functions by full name and trailing segment chain.
type symbolTable struct {
	// sources says which file a declared name reads from. More than one file
	// can hold a name, because the tree is the whole source root, larger than
	// one program. No check has refused the second declaration yet, and
	// reporting both is more use than picking one.
	sources map[string][]symbolSource

	// exact answers a name spelled in full.
	exact map[string][]string

	// suffix answers a trailing segment chain. `main`, `Handler\main` reach
	// `App\Handler\main`. The whole name is not a key here; exact holds that.
	// Keeping them apart lets a tree declaring `main` and `App\Handler\main`
	// answer the first without a tie.
	suffix map[string][]string
}

// symbolIndex returns the tree's function index, rebuilding it when the include cache changes.
func (rt *Runtime) symbolIndex() *symbolTable {
	rt.scanTree()
	if rt.symbols != nil && !rt.includeCache.changedSince(rt.symbolsVersion) {
		return rt.symbols
	}

	programs, version := rt.includeCache.snapshot()
	table := &symbolTable{
		sources: make(map[string][]symbolSource, len(programs)),
		exact:   make(map[string][]string, len(programs)),
		suffix:  make(map[string][]string, len(programs)),
	}
	for path, program := range programs {
		for _, stmt := range program.Stmts {
			decl, ok := stmt.(*model.FuncDecl)
			// A `function Class::method` declaration is a method and is not
			// callable by name.
			if !ok || decl.Class != "" {
				continue
			}
			// table.index fills the name maps the first time a spelling
			// arrives. A name two files declare becomes one entry in each,
			// with two sources.
			if _, seen := table.sources[decl.Name]; !seen {
				table.index(decl.Name)
			}
			table.sources[decl.Name] = append(table.sources[decl.Name], symbolSource{path: path, program: program})
		}
	}

	rt.symbols, rt.symbolsVersion = table, version
	return table
}

// index records a name's full spelling and each trailing segment chain.
func (t *symbolTable) index(name string) {
	folded := strings.ToLower(name)
	t.exact[folded] = append(t.exact[folded], name)
	for at := 0; ; {
		cut := strings.Index(folded[at:], "\\")
		if cut < 0 {
			return
		}
		at += cut + 1
		t.suffix[folded[at:]] = append(t.suffix[folded[at:]], name)
	}
}

// declaredUserFunc returns the declared spelling of name, comparing case-insensitively.
func (rt *Runtime) declaredUserFunc(name string) (string, bool) {
	if _, ok := rt.userFns[name]; ok {
		return name, true
	}
	for declared := range rt.userFns {
		if strings.EqualFold(declared, name) {
			return declared, true
		}
	}
	return "", false
}

// scanTree precompiles the source root into an empty include cache once.
func (rt *Runtime) scanTree() {
	if rt.lookupScanned {
		return
	}
	rt.lookupScanned = true
	if rt.opts.RootFS == nil || rt.includeCache.Len() > 0 {
		return
	}
	Precompiler{Root: rt.opts.RootFS, Includes: rt.includeCache, GoRuntime: rt.opts.GoRuntime()}.Run()
}

// matchSymbols returns names spelling symName in full, else trailing-segment matches.
func matchSymbols(names []string, symName string) []string {
	var exact []string
	for _, name := range names {
		if strings.EqualFold(name, symName) {
			exact = append(exact, name)
		}
	}
	if len(exact) > 0 {
		return exact
	}

	var suffix []string
	tail := "\\" + symName
	for _, name := range names {
		if len(name) > len(tail) && strings.EqualFold(name[len(name)-len(tail):], tail) {
			suffix = append(suffix, name)
		}
	}
	return suffix
}

// resultShape holds a signature's value type and whether it takes an error.
type resultShape struct {
	value reflect.Type
	err   bool
}

// signatureResults reads T's result shape, refusing a third result or a non-error second.
func signatureResults(typ reflect.Type) (resultShape, error) {
	switch typ.NumOut() {
	case 0:
		return resultShape{}, nil
	case 1:
		if typ.Out(0) == errorType {
			return resultShape{err: true}, nil
		}
		return resultShape{value: typ.Out(0)}, nil
	case 2:
		if typ.Out(1) != errorType {
			return resultShape{}, fmt.Errorf("a two-result signature must end in error, %s ends in %s", typ, typ.Out(1))
		}
		return resultShape{value: typ.Out(0), err: true}, nil
	default:
		return resultShape{}, fmt.Errorf("a signature returns at most one value and an error, %s returns %d", typ, typ.NumOut())
	}
}

// lookupArgs widens call arguments to PHP values, spreading a variadic tail into elements.
func lookupArgs(typ reflect.Type, in []reflect.Value) []any {
	fixed := len(in)
	total := fixed
	if typ.IsVariadic() && fixed > 0 {
		fixed--
		total = fixed + in[len(in)-1].Len()
	}

	args := make([]any, 0, total)
	for i := range fixed {
		args = append(args, phpArg(in[i]))
	}
	if total != fixed {
		tail := in[len(in)-1]
		for i := range tail.Len() {
			args = append(args, phpArg(tail.Index(i)))
		}
	}
	return args
}

// phpArg widens a predeclared numeric to int64 or float64, keeping a named type.
func phpArg(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Type().PkgPath() != "" {
		return v.Interface()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(v.Uint())
	case reflect.Float32, reflect.Float64:
		return v.Float()
	}
	return v.Interface()
}

// lookupReturn fills the results, sending an unreportable error to the runtime sink.
func (rt *Runtime) lookupReturn(typ reflect.Type, shape resultShape, value any, err error) []reflect.Value {
	out := make([]reflect.Value, typ.NumOut())
	if shape.value != nil {
		result := reflect.Zero(shape.value)
		if err == nil {
			converted, convErr := convertResult(shape.value, value)
			if convErr != nil {
				err = convErr
			} else {
				result = converted
			}
		}
		out[0] = result
	}
	if !shape.err {
		rt.RecordError(err)
		return out
	}
	// Addressed through the variable and not through the value and the slot
	// holds the error interface MakeFunc declared in place of the concrete type
	// behind it.
	out[len(out)-1] = reflect.ValueOf(&err).Elem()
	return out
}

// convertResult coerces value to want, using PHP's scalar coercions and coerceArg.
func convertResult(want reflect.Type, value any) (reflect.Value, error) {
	if want.Kind() == reflect.Interface && want.NumMethod() == 0 {
		if value == nil {
			return reflect.Zero(want), nil
		}
		return reflect.ValueOf(value), nil
	}
	if want.PkgPath() == "" {
		switch want.Kind() {
		case reflect.Bool:
			return reflect.ValueOf(phpTruthy(value)), nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return reflect.ValueOf(toInt(value)).Convert(want), nil
		case reflect.Float32, reflect.Float64:
			return reflect.ValueOf(toFloat(value)).Convert(want), nil
		case reflect.String:
			return reflect.ValueOf(phpString(value)), nil
		case reflect.Slice:
			// A string filling a []byte is a conversion, not a walk, and
			// coerceArg below already performs it.
			if modelutil.IsCollection(value) {
				return convertSlice(want, value)
			}
		case reflect.Map:
			if modelutil.IsCollection(value) {
				return convertMap(want, value)
			}
		}
	}
	if converted, ok := coerceArg(want, value); ok {
		return converted, nil
	}
	return reflect.Value{}, fmt.Errorf("cannot use %T as %s", value, want)
}

// convertSlice walks a PHP array into a typed slice, discarding keys and passing an assignable one unchanged.
func convertSlice(want reflect.Type, value any) (reflect.Value, error) {
	if rv := reflect.ValueOf(value); rv.IsValid() && rv.Type().AssignableTo(want) {
		return rv, nil
	}
	size, _ := modelutil.LenValues(value)
	out := reflect.MakeSlice(want, 0, size)
	elem := want.Elem()

	var err error
	modelutil.RangeValues(value, func(_, item any) bool {
		converted, convErr := convertResult(elem, item)
		if convErr != nil {
			err = convErr
			return false
		}
		out = reflect.Append(out, converted)
		return true
	})
	if err != nil {
		return reflect.Value{}, err
	}
	return out, nil
}

// convertMap walks a PHP array into a typed map, keys in its kind.
func convertMap(want reflect.Type, value any) (reflect.Value, error) {
	if rv := reflect.ValueOf(value); rv.IsValid() && rv.Type().AssignableTo(want) {
		return rv, nil
	}
	size, _ := modelutil.LenValues(value)
	out := reflect.MakeMapWithSize(want, size)
	keyType, elem := want.Key(), want.Elem()

	var err error
	modelutil.RangeValues(value, func(key, item any) bool {
		convertedKey, keyErr := convertResult(keyType, key)
		if keyErr != nil {
			err = keyErr
			return false
		}
		converted, convErr := convertResult(elem, item)
		if convErr != nil {
			err = convErr
			return false
		}
		out.SetMapIndex(convertedKey, converted)
		return true
	})
	if err != nil {
		return reflect.Value{}, err
	}
	return out, nil
}
