package runner

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// Lookup resolves symName to a PHP function and returns it as T, a Go function
// type. It is plugin.Lookup over a source tree: the host names a symbol and a
// signature, and gets back something it can call.
//
//	handle, err := runner.Lookup[func(*http.Request) bool](rt, "App\\Handler\\main")
//	if err == nil {
//		ok := handle(request)
//	}
//
// The symbol may be declared anywhere in the tree rt serves, not only in a file
// the host loaded. A name is matched against the functions already declared on
// rt and against every program in its include cache; a cache that is empty and
// a source root that is not runs a Precompiler pass first, so a runtime with
// nothing loaded still resolves. The parser qualifies a free function with the
// namespace its file declares, so "App\Handler\main" is the whole name and a
// bare "main" matches it by its trailing segment. A name that matches nothing,
// or more than one declaration, is a *LookupError naming what it matched.
//
// T must be a function type returning at most one value and an optional
// trailing error. The shape is checked here rather than at the call, so a
// signature the symbol cannot fill fails where the host binds it.
//
// An invocation carries the arguments and nothing else. No runner.Context is
// registered, so $_GET, $_POST and $_SERVER are absent rather than empty, and
// no file body runs: the declaring program is hoisted for its declarations. The
// arguments arrive as the Go values they are, which is what makes an
// *http.Request handed in reachable as HTTP\Request. Output goes where the
// runtime's does.
//
// The returned function belongs to rt, and a Runtime serves one goroutine.
// A host calling one concurrently builds a runtime per goroutine and shares the
// include and expression caches between them, which is what the server does per
// request.
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
	// Symbol is the name the host asked for, as it spelled it.
	Symbol string

	// Candidates holds what the name matched when it matched too much. It is
	// empty for every other reason, which is what separates "there is no such
	// function" from "say which one".
	Candidates []string

	// Reason says what went wrong in the words the host needs.
	Reason string
}

// Error names the symbol and the reason, and lists what the name matched when
// it matched too much, so a host is told which spelling to use.
func (e *LookupError) Error() string {
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("lookup %s: %s", e.Symbol, e.Reason)
	}
	return fmt.Sprintf("lookup %s: %s: %s", e.Symbol, e.Reason, strings.Join(e.Candidates, ", "))
}

// lookup is the type-erased half of Lookup. The generic wrapper is one line so
// that a program looking up a dozen signatures instantiates a dozen one-line
// functions over one body rather than a dozen copies of this.
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
		// Resolution found the declaration and hoisting installed it, so a miss
		// here is the runtime disagreeing with itself rather than a bad name.
		return reflect.Value{}, &LookupError{Symbol: symName, Reason: "resolved to " + name + ", which is not in the function table"}
	}

	return reflect.MakeFunc(typ, func(in []reflect.Value) []reflect.Value {
		value, callErr := rt.invokeEntry(entry, lookupArgs(in, typ), nil)
		return rt.lookupReturn(typ, shape, value, callErr)
	}), nil
}

// symbolSource names where a declaration was read from.
type symbolSource struct {
	path    string
	program *model.Program
}

// resolveSymbol turns the name a host asked for into the name the function
// table holds, installing the declaration if it is only in the tree so far.
//
// The three passes are ordered rather than merged, so a tree holding both
// `main` and `App\Handler\main` answers the first for "main" and needs no
// disambiguation. Only the last one scans anything: it covers a function
// declared by a program the source root does not hold, which is a runtime the
// host ran a string on rather than one serving a tree.
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
		// PHP's redeclaration error the moment the other was reached, so the
		// host is told which files disagree instead.
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
	// The file counts as included from here. Its declarations are live, so an
	// application that require_once's it must not declare them a second time;
	// without this the tree a handler was bound out of could not be loaded by
	// the application that owns it.
	//
	// What it costs is the file's top-level code, which a later require_once
	// now skips. A file that declares handlers and also does work at include
	// time has to be required before the lookup rather than after.
	rt.markIncluded(sources[0].path)
	return found, nil
}

// symbolTable is the tree's free functions, indexed for the two ways a host
// spells one. Both name maps are keyed by the folded name, because PHP compares
// a function name case-insensitively, and both answer with names as declared.
type symbolTable struct {
	// sources says where a declared name was read from. A name can be held by
	// more than one file: the tree is the whole source root rather than one
	// program, so nothing has refused the second declaration yet, and reporting
	// both is more use than picking one.
	sources map[string][]symbolSource

	// exact answers a name spelled in full.
	exact map[string][]string

	// suffix answers a trailing segment chain, so `main` and `Handler\main`
	// both reach `App\Handler\main`. The whole name is not a key here; exact
	// holds that, and keeping them apart is what lets a tree declaring both
	// `main` and `App\Handler\main` answer the first without a tie.
	suffix map[string][]string
}

// symbolIndex indexes the free functions the loaded tree declares.
//
// The table is kept until the cache it was built from is written to again. It
// is a walk of every statement of every program in the tree, so rebuilding it
// per lookup made binding a handler cost the size of the application: 546us
// and 470KB over eight hundred files, paid again for the next handler. A host
// serving concurrently binds per runtime and therefore per goroutine, which is
// often per request, so that is not a startup cost it could have absorbed.
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
			// The name maps are filled the first time a spelling is seen, so a
			// name two files declare is one entry in each and two sources.
			if _, seen := table.sources[decl.Name]; !seen {
				table.index(decl.Name)
			}
			table.sources[decl.Name] = append(table.sources[decl.Name], symbolSource{path: path, program: program})
		}
	}

	rt.symbols, rt.symbolsVersion = table, version
	return table
}

// index records one declared name under its full spelling and under every
// trailing segment chain of it.
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

// declaredUserFunc answers the spelling a PHP function is declared under on
// this runtime, for a name given in full.
//
// The fold scan behind it is the one lookupEntry performs for every call PHP
// makes under a spelling the table does not hold, and it is reached only when
// the tree has no function of that name.
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

// scanTree parses the source root into the include cache the first time a
// lookup needs it, so a host that has loaded nothing still resolves a symbol.
//
// A cache with anything in it is left alone: the host either precompiled or
// served requests off it, and both fill it with the same programs this would.
func (rt *Runtime) scanTree() {
	if rt.lookupScanned {
		return
	}
	rt.lookupScanned = true
	if rt.opts.RootFS == nil || rt.includeCache.Len() > 0 {
		return
	}
	Precompiler{Root: rt.opts.RootFS, Includes: rt.includeCache}.Run()
}

// matchSymbols returns the declared names symName selects: the ones spelling it
// in full, and failing that the ones ending in it after a namespace separator.
//
// The two passes are ordered rather than merged, so a tree holding both
// `main` and `App\Handler\main` answers the first for "main" and needs no
// disambiguation. Both compare case-insensitively, which is how PHP compares a
// function name.
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

// resultShape is what the requested signature does with what the function
// returns: the type its value lands in, and whether it has somewhere to put an
// error.
type resultShape struct {
	value reflect.Type
	err   bool
}

// signatureResults reads the shape off T and refuses the ones a PHP function
// cannot fill. PHP returns one value; a second Go result is the error, and a
// third has nothing to be.
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

// lookupArgs renders the call's arguments as the values a PHP scope holds. A
// variadic tail is spread, because PHP counts the arguments it was passed and a
// slice in the last slot is one of them.
func lookupArgs(in []reflect.Value, typ reflect.Type) []any {
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

// phpArg widens one argument to the value set the interpreter operates on.
// bindParams stores what it is handed verbatim, so this is the only place a Go
// int becomes the int64 PHP arithmetic reads as a number.
//
// Only the predeclared numeric types are widened. A named scalar keeps its
// name: toInt and phpString already read time.Month and time.Duration as the
// numbers they are, and a binding taking one back needs the type intact. Every
// other value is passed through, which is what makes an *http.Request the
// script's own HTTP\Request rather than a copy of one.
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

// lookupReturn fills the signature's results from what the function returned.
//
// A signature with no error result has nowhere to report a failure, so the
// error goes to the runtime's own sink, where the error of a script that failed
// without the host asking for one goes.
func (rt *Runtime) lookupReturn(typ reflect.Type, shape resultShape, value any, err error) []reflect.Value {
	out := make([]reflect.Value, typ.NumOut())
	if shape.value != nil {
		result := reflect.Zero(shape.value)
		if err == nil {
			converted, convErr := convertResult(value, shape.value)
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
	// Addressed through the variable rather than through the value, so the slot
	// holds the error interface MakeFunc declared instead of the concrete type
	// behind it.
	out[len(out)-1] = reflect.ValueOf(&err).Elem()
	return out
}

// convertResult renders a returned PHP value as the Go type the signature
// declared.
//
// The predeclared scalar kinds go through PHP's own coercions, so a function
// returning 1 fills a bool result the way `if (1)` reads it and a function
// returning "3" fills an int result with 3. Everything else goes through
// coerceArg, which is the conversion every Go binding's arguments already pass.
func convertResult(value any, want reflect.Type) (reflect.Value, error) {
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
			if model.IsCollection(value) {
				return convertSlice(value, want)
			}
		case reflect.Map:
			if model.IsCollection(value) {
				return convertMap(value, want)
			}
		}
	}
	if converted, ok := coerceArg(value, want); ok {
		return converted, nil
	}
	return reflect.Value{}, fmt.Errorf("cannot use %T as %s", value, want)
}

// convertSlice walks a PHP array into a typed slice, keys discarded. A binding
// returning the cheap shape for its data (a []string, a []map[string]any) is
// assignable and is handed over as it is.
func convertSlice(value any, want reflect.Type) (reflect.Value, error) {
	if rv := reflect.ValueOf(value); rv.IsValid() && rv.Type().AssignableTo(want) {
		return rv, nil
	}
	size, _ := model.LenValues(value)
	out := reflect.MakeSlice(want, 0, size)
	elem := want.Elem()

	var err error
	model.RangeValues(value, func(_, item any) bool {
		converted, convErr := convertResult(item, elem)
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

// convertMap walks a PHP array into a typed map. The key is rendered the way
// PHP renders an array key in the target's kind, so a string-keyed map reads
// the names and an integer-keyed one reads the offsets.
func convertMap(value any, want reflect.Type) (reflect.Value, error) {
	if rv := reflect.ValueOf(value); rv.IsValid() && rv.Type().AssignableTo(want) {
		return rv, nil
	}
	size, _ := model.LenValues(value)
	out := reflect.MakeMapWithSize(want, size)
	keyType, elem := want.Key(), want.Elem()

	var err error
	model.RangeValues(value, func(key, item any) bool {
		convertedKey, keyErr := convertResult(key, keyType)
		if keyErr != nil {
			err = keyErr
			return false
		}
		converted, convErr := convertResult(item, elem)
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
