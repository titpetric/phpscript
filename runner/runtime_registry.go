package runner

import (
	"fmt"
	"maps"
	"path"
	goruntime "runtime"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner/coverage"
)

// This file is the runtime's symbol tables and the API that fills and reads
// them: functions, classes, interfaces, constructors, constants, globals,
// autoloaders and the shutdown list, with the introspection snapshots the
// diagnostic surface answers from.

// RegisterConstructor binds a class name to a Go constructor. Example:
//
//	rt.RegisterConstructor("Storage", func(ctx context.Context) (Storage, error) { ... }).
//	// PHP:  $storage = new Storage;   // == storage, err := NewStorage(ctx).
func (rt *Runtime) RegisterConstructor(name string, ctor any) {
	rt.constructors[name] = &funcEntry{fn: ctor}
}

// LookupConstructor returns the host constructor registered for name.
func (rt *Runtime) LookupConstructor(name string) (any, bool) {
	if entry, ok := rt.constructors[name]; ok {
		return entry.fn, true
	}
	return nil, false
}

// SetCoverage installs a statement-coverage collector.
func (rt *Runtime) SetCoverage(c *coverage.Collector) { rt.coverage = c }

// SetIncludeCache installs a cache shared by the runtimes of one source root.
func (rt *Runtime) SetIncludeCache(cache *IncludeCache) { rt.includeCache = cache }

// SetExprCache installs a source-keyed cache any runtime may share.
func (rt *Runtime) SetExprCache(cache *ExprCache) { rt.exprCache = cache }

// SetGlobal seeds a variable into the global scope before execution. Useful for
// injecting request data (the README's $_SERVER gray area) or, in tests, an
// input value.
func (rt *Runtime) SetGlobal(name string, val any) {
	if rt.auto.Set(name, val) {
		return
	}
	rt.globals[name] = val
}

// PutEnv writes one variable into the script environment, backing putenv(),
// copying the frozen snapshot first so the write dies with its program.
func (rt *Runtime) PutEnv(name, value string) {
	rt.ownEnv()
	rt.Env[name] = value
}

// UnsetEnv removes one variable from the script environment, the bare-name
// spelling of putenv(). It copies the frozen snapshot first, as PutEnv does.
func (rt *Runtime) UnsetEnv(name string) {
	rt.ownEnv()
	delete(rt.Env, name)
}

// ownEnv makes Env this program's to write.
func (rt *Runtime) ownEnv() {
	if rt.envShared && rt.frozenEnv != nil {
		rt.Env = maps.Clone(rt.frozenEnv)
		rt.envShared = false
	}
}

// SetConst registers a PHP constant (e.g. define("FOO", 1) or a built-in like
// T_VARIABLE). Constants are visible in every scope, including inside functions
// and methods, unlike globals, which PHP confines to the global scope.
func (rt *Runtime) SetConst(name string, val any) {
	if rt.constsShared && rt.frozenConsts != nil {
		rt.constants = maps.Clone(rt.frozenConsts)
		rt.constsShared = false
	}
	rt.constants[name] = val
}

// RegisterShutdown appends a callback the program's exit runs, in order.
func (rt *Runtime) RegisterShutdown(callback any) {
	rt.shutdown = append(rt.shutdown, callback)
}

// Const returns a registered constant and whether the table holds it. The
// magic constants are not among them, as in php.
func (rt *Runtime) Const(name string) (any, bool) {
	v, ok := rt.constants[name]
	return v, ok
}

// entrypointConst answers __FILE__ and __DIR__ for source that named no file.
//
// parser.ParseFile compiles both to literals and a program read from the
// filesystem never asks. That leaves Runtime.Load, which takes source with
// no file behind it, and there the entrypoint is the best answer there is.
//
// They are not in the constant table. php resolves them when it compiles and
// defined("__FILE__") is false there and get_defined_constants() omits them;
// holding them as constants reported the opposite, and cost a copy of the whole
// table per run because the first write after FreezeStdlib clones it.
func (rt *Runtime) entrypointConst(name string) (any, bool) {
	if rt.entrypoint == "" {
		return nil, false
	}
	switch name {
	case "__FILE__":
		return rt.entrypoint, true
	case "__DIR__":
		return path.Dir(rt.entrypoint), true
	}
	return nil, false
}

// RegisterFunc forwards a Go function (or any callable) into the VM under name.
// This is the shim mechanism: e.g. rt.RegisterFunc("strlen", func(s string) int
// { return len(s) }) makes `strlen($x)` work in script expressions.
func (rt *Runtime) RegisterFunc(name string, fn any) {
	// This invalidates no entry: evaluation environments resolve the table per
	// call (see installFunc), the direct compiler reads no type env, and the
	// entry builds its invoker on first dispatch and a re-registration is a
	// fresh entry with a fresh invoker.
	rt.funcs[name] = &funcEntry{fn: fn}
}

// LookupFunc returns the host function registered for name. Introspection
// tooling uses it to reflect over a binding's Go signature; a PHP user-defined
// function resolves too, as whatever callable the runtime stored for it.
func (rt *Runtime) LookupFunc(name string) (any, bool) {
	if entry, ok := rt.funcs[name]; ok {
		return entry.fn, true
	}
	return nil, false
}

func (rt *Runtime) registerUserFunc(name string, fn any) {
	rt.RegisterFunc(name, fn)
	rt.userFns[name] = struct{}{}
}

// FunctionExists reports whether name resolves to a host shim or a PHP
// function, backing `function_exists`.
func (rt *Runtime) FunctionExists(name string) bool {
	_, ok := rt.lookupFunc(strings.TrimPrefix(name, "\\"))
	return ok
}

func (rt *Runtime) lookupFunc(name string) (any, bool) {
	if entry, ok := rt.lookupEntry(name); ok {
		return entry.fn, true
	}
	return nil, false
}

// lookupEntry resolves a function-table entry: exact name, then the
// case-insensitive scan PHP's function names require.
func (rt *Runtime) lookupEntry(name string) (*funcEntry, bool) {
	if entry, ok := rt.funcs[name]; ok {
		return entry, true
	}
	for functionName, entry := range rt.funcs {
		if strings.EqualFold(functionName, name) {
			return entry, true
		}
	}
	return nil, false
}

// DefinedFunctions returns stable snapshots of registered host/internal and
// PHP user-defined function names.
func (rt *Runtime) DefinedFunctions() (internal, user []string) {
	for name := range rt.funcs {
		if _, ok := rt.userFns[name]; ok {
			user = append(user, name)
		} else {
			internal = append(internal, name)
		}
	}
	sort.Strings(internal)
	sort.Strings(user)
	return internal, user
}

// DefinedConstants returns a stable snapshot of all runtime constants.
func (rt *Runtime) DefinedConstants() map[string]any {
	return maps.Clone(rt.constants)
}

// DeclaredClasses returns the names of PHP classes and host-backed constructor
// classes available to the runtime. PHP does not guarantee ordering;
// phpscript sorts the snapshot for deterministic diagnostics.
func (rt *Runtime) DeclaredClasses() []string {
	names := make(map[string]struct{}, len(rt.classes)+len(rt.constructors))
	for name := range rt.classes {
		names[name] = struct{}{}
	}
	for name := range rt.constructors {
		names[name] = struct{}{}
	}
	// stdClass is not in either table, and get_declared_classes() lists it.
	names[model.StdClass().Name] = struct{}{}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// PHPInfo prints a compact phpinfo-style text report for the phpscript
// runtime. It reports runtime facts; the PHP extensions phpscript does not
// carry are outside it.
func (rt *Runtime) PHPInfo() error {
	internal, user := rt.DefinedFunctions()
	_, err := fmt.Fprintf(rt.Output(), "phpscript\n\nRuntime => phpscript\nSAPI => %s\nGo Version => %s\nOperating System => %s\nArchitecture => %s\nInclude Path => %s\nWorking Directory => %s\nInternal Functions => %d\nUser Functions => %d\nDeclared Classes => %d\nDefined Constants => %d\n",
		rt.SAPI(), goruntime.Version(), goruntime.GOOS, goruntime.GOARCH,
		rt.IncludePath(), rt.WorkDir(), len(internal), len(user),
		len(rt.DeclaredClasses()), len(rt.constants))
	if err != nil {
		return err
	}
	report := rt.infoReport()
	if report == "" {
		return nil
	}
	_, err = fmt.Fprint(rt.Output(), report)
	return err
}

// RegisterClass adds a resolved class to the class table so `new Name` works.
func (rt *Runtime) RegisterClass(c *model.Class) {
	if existing, ok := rt.classes[c.Name]; ok && existing != nil {
		if existing.Methods == nil {
			existing.Methods = map[string]*model.FuncDecl{}
		}
		for name, method := range c.Methods {
			existing.Methods[name] = method
		}
		if len(c.Fields) > 0 {
			existing.Fields = c.Fields
		}
		return
	}
	rt.classes[c.Name] = c
}

// RegisterAutoloader appends or prepends a callback to the SPL autoload queue.
// The callback receives a fully-qualified class name without a leading slash.
func (rt *Runtime) RegisterAutoloader(callback any, prepend bool) {
	if prepend {
		rt.autoloaders = append([]any{callback}, rt.autoloaders...)
		return
	}
	rt.autoloaders = append(rt.autoloaders, callback)
}

// SetIncludePath sets the path list used by the default SPL autoloader and
// returns its previous value.
func (rt *Runtime) SetIncludePath(value string) string {
	old := rt.includePath
	rt.includePath = value
	return old
}

// IncludePath returns the current SPL include path.
func (rt *Runtime) IncludePath() string { return rt.includePath }

// ClassExists reports whether a PHP or host-backed class exists. If autoload is
// true, the registered autoloaders run for a missing class.
func (rt *Runtime) ClassExists(name string, autoload bool) (bool, error) {
	name = strings.TrimPrefix(name, "\\")
	if rt.hasClass(name) {
		return true, nil
	}
	if !autoload {
		return false, nil
	}
	if err := rt.autoload(name, rt.newScope()); err != nil {
		return false, err
	}
	return rt.hasClass(name), nil
}

// MethodExists reports whether a declared PHP class has a method by that name,
// using PHP's case-insensitive method lookup. It answers only for interpreted
// classes; its caller reflects over a host-backed one.
func (rt *Runtime) MethodExists(class, method string) bool {
	decl, ok := rt.lookupClass(strings.TrimPrefix(class, "\\"))
	if !ok {
		return false
	}
	_, ok = lookupPHPMethod(decl, method)
	return ok
}

func (rt *Runtime) hasClass(name string) bool {
	if _, ok := rt.lookupConstructor(name); ok {
		return true
	}
	_, ok := rt.lookupClass(name)
	return ok
}

func (rt *Runtime) lookupClass(name string) (*model.Class, bool) {
	if c, ok := rt.classes[name]; ok {
		return c, true
	}
	for className, c := range rt.classes {
		if strings.EqualFold(className, name) {
			return c, true
		}
	}
	// stdClass answers last so a script that declares its own shadows it,
	// and from outside the table so that clearing the table between runs does
	// not take it away. See model.StdClass.
	if strings.EqualFold(name, "stdClass") {
		return model.StdClass(), true
	}
	return nil, false
}

// lookupInterface resolves a declared interface by name, with the same
// case-insensitive fallback PHP applies to class-like names.
func (rt *Runtime) lookupInterface(name string) (*model.InterfaceDecl, bool) {
	if id, ok := rt.interfaces[name]; ok {
		return id, true
	}
	for interfaceName, id := range rt.interfaces {
		if strings.EqualFold(interfaceName, name) {
			return id, true
		}
	}
	return nil, false
}

// InterfaceExists reports whether the interface table holds name. If autoload
// is true, the registered autoloaders run for a missing one; composer's
// autoloader maps interfaces and classes through the same PSR-4 table.
func (rt *Runtime) InterfaceExists(name string, autoload bool) (bool, error) {
	name = strings.TrimPrefix(name, "\\")
	if _, ok := rt.lookupInterface(name); ok {
		return true, nil
	}
	if !autoload {
		return false, nil
	}
	if err := rt.autoload(name, rt.newScope()); err != nil {
		return false, err
	}
	_, ok := rt.lookupInterface(name)
	return ok, nil
}

func (rt *Runtime) lookupConstructor(name string) (*funcEntry, bool) {
	if ctor, ok := rt.constructors[name]; ok {
		return ctor, true
	}
	for className, ctor := range rt.constructors {
		if strings.EqualFold(className, name) {
			return ctor, true
		}
	}
	return nil, false
}

// OnError installs the handler evaluation errors route to, aborting nobody.
func (rt *Runtime) OnError(fn func(error)) {
	rt.errorHandler = fn
}

// OnReset registers fn to run between programs.
func (rt *Runtime) OnReset(fn func()) {
	rt.resets = append(rt.resets, fn)
}
