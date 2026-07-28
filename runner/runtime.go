package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/titpetric/phpscript/model"
)

// phpSuperglobals are visible in every scope without a `global` declaration.
var phpSuperglobals = map[string]struct{}{
	"_COOKIE": {}, "_ENV": {}, "_FILES": {}, "_GET": {},
	"_POST": {}, "_REQUEST": {}, "_SERVER": {}, "_SESSION": {},
	"_PATH": {},
}

// Runtime executes parsed PHP statements and evaluates transpiled expressions
// with registered functions, classes, constructors, and runtime state.
type Runtime struct {
	out     io.Writer
	funcs   map[string]any
	userFns map[string]struct{}
	wrapped map[string]func(...any) (any, error)
	classes map[string]*model.Class

	// constructors maps a class name to a Go constructor function, so PHP's
	// `new Name` instantiates a native Go value rather than a model.Object. This
	// is the bridge for "bring your own type": a constructor like
	// func(ctx context.Context) (Storage, error) makes `$s = new Storage;`
	// behave like `s, err := NewStorage(ctx)` (the error surfaces as a throw).
	constructors map[string]any
	autoloaders  []any
	includePath  string

	// ctx is the request/lifecycle context auto-injected into any registered
	// callable (constructor, method, function) whose first parameter is a
	// context.Context — mirroring vuego's wrapContextFunc. It lets PHP call
	// these symbols without supplying the context argument explicitly.
	ctx context.Context

	errorHandler func(error)
	include      IncludeFunc
	globals      map[string]any

	// constants holds PHP constants (define()/named T_* etc.). Unlike globals,
	// constants are visible in every scope — including inside functions and
	// methods — matching PHP's constant semantics. Bare-identifier lookups that
	// miss the current scope fall back here (see Eval).
	constants map[string]any

	// opts holds runtime source root, working directory, and write policy.
	opts         Options
	includeCache *IncludeCache
	included     []string

	// classConsts caches evaluated class constants (Class::NAME) per class.
	classConsts map[string]map[string]any

	mu        sync.Mutex
	cache     map[string]*vm.Program // expr source -> compiled program
	exprCache *ExprCache
	compiled  map[model.Expr]*compiledExpr
	helpers   map[string]func(...any) (any, error)
}

// evalEnvPool is shared by runtimes because HTTP integrations commonly create
// one Runtime per request. A per-Runtime pool never survived long enough to
// reuse its first environment map. releaseEnv removes every value (including
// closures over the Runtime and Scope) before returning a map to this pool.
var evalEnvPool = sync.Pool{
	New: func() any { return make(map[string]any, 128) },
}

// ExitError is returned when PHP die()/exit() interrupts script execution.
type ExitError struct {
	Code int
}

// Error returns a formatted PHP exit status.
func (e *ExitError) Error() string {
	return fmt.Sprintf("exit(%d)", e.Code)
}

// SAPI returns the configured SAPI string.
func (rt *Runtime) SAPI() string {
	return rt.opts.SAPI
}

// IsExit reports whether err was caused by PHP die()/exit().
func IsExit(err error) (*ExitError, bool) {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr, true
	}
	return nil, false
}

// Exit interrupts execution with a PHP exit status.
func (rt *Runtime) Exit(code int) error {
	return &ExitError{Code: code}
}

// New returns a Runtime that writes echo output to w (defaults to os.Stdout).
func New(w io.Writer, opts Options) *Runtime {
	if w == nil {
		w = os.Stdout
	}
	opts.WorkDir = cleanFSPath(opts.WorkDir)
	rt := &Runtime{
		out:          w,
		opts:         opts,
		includeCache: NewIncludeCache(),
		funcs:        map[string]any{},
		userFns:      map[string]struct{}{},
		wrapped:      map[string]func(...any) (any, error){},
		classes:      map[string]*model.Class{},
		constructors: map[string]any{},
		includePath:  ".",
		ctx:          context.Background(),
		globals:      map[string]any{},
		constants:    map[string]any{},
		classConsts:  map[string]map[string]any{},
		exprCache:    NewExprCache(),
		helpers: map[string]func(...any) (any, error){
			"__bool":   adapt(phpTruthy),
			"__concat": adapt(helperConcat),
			"__pair":   adapt(helperPair),
			"__array":  adapt(helperArray),
			"__index":  adapt(helperIndex),
			"__cast":   adapt(helperCast),
			"__arith":  adapt(phpArith),
		},
	}
	return rt
}

// SetContext installs the lifecycle context auto-injected into registered Go
// callables whose first parameter is a context.Context (constructors, methods,
// functions). Defaults to context.Background().
func (rt *Runtime) SetContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	rt.ctx = ctx
}

// Context returns the configured lifecycle context.
func (rt *Runtime) Context() context.Context {
	return rt.ctx
}

// RegisterConstructor binds a class name to a Go constructor so `new Name` in
// PHP instantiates a native Go value. The constructor may take a leading
// context.Context (auto-injected) and may return a trailing error, which is
// surfaced to the interpreter as a thrown error. Example:
//
//	rt.RegisterConstructor("Storage", func(ctx context.Context) (Storage, error) { ... }).
//	// PHP:  $storage = new Storage;   // == storage, err := NewStorage(ctx).
func (rt *Runtime) RegisterConstructor(name string, ctor any) {
	rt.constructors[name] = ctor
}

// SetIncludeCache installs a shared include cache. A cache must only be shared
// by runtimes whose include paths resolve within the same source-root namespace.
// Passing nil disables include caching.
func (rt *Runtime) SetIncludeCache(cache *IncludeCache) { rt.includeCache = cache }

// SetExprCache installs a source-keyed compiled-program cache that is safe to
// share across runtimes. AST-specific expression metadata remains runtime-local.
// Passing nil disables cross-runtime expression caching.
func (rt *Runtime) SetExprCache(cache *ExprCache) { rt.exprCache = cache }

// FS returns the configured source root (or nil).
func (rt *Runtime) FS() fs.FS { return rt.opts.RootFS }

// WorkDir returns the configured working directory inside the source root.
func (rt *Runtime) WorkDir() string { return rt.opts.WorkDir }

// WritablePaths returns the configured writable path whitelist.
func (rt *Runtime) WritablePaths() []string { return append([]string(nil), rt.opts.WritablePaths...) }

// IncludedFiles returns the cleaned dirFS filenames included by this runtime.
func (rt *Runtime) IncludedFiles() []string { return append([]string(nil), rt.included...) }

// SetGlobal seeds a variable into the global scope before execution. Useful for
// injecting request data (the README's $_SERVER gray area) or, in tests, an
// input value.
func (rt *Runtime) SetGlobal(name string, val any) {
	rt.globals[name] = val
}

// SetConst registers a PHP constant (e.g. define("FOO", 1) or a built-in like
// T_VARIABLE). Constants are visible in every scope, including inside functions
// and methods — unlike globals, which PHP confines to the global scope.
func (rt *Runtime) SetConst(name string, val any) {
	rt.constants[name] = val
}

// Const returns a registered constant value and whether it is defined.
func (rt *Runtime) Const(name string) (any, bool) {
	v, ok := rt.constants[name]
	return v, ok
}

// RegisterFunc forwards a Go function (or any callable) into the VM under name.
// This is the shim mechanism: e.g. rt.RegisterFunc("strlen", func(s string) int
// { return len(s) }) makes `strlen($x)` work in transpiled code.
func (rt *Runtime) RegisterFunc(name string, fn any) {
	rt.funcs[name] = fn
	rt.wrapped[name] = adapt(fn)
}

func (rt *Runtime) registerUserFunc(name string, fn any) {
	rt.RegisterFunc(name, fn)
	rt.userFns[name] = struct{}{}
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
// classes currently available to the runtime. PHP does not guarantee ordering;
// phpscript sorts the snapshot for deterministic diagnostics.
func (rt *Runtime) DeclaredClasses() []string {
	names := make(map[string]struct{}, len(rt.classes)+len(rt.constructors))
	for name := range rt.classes {
		names[name] = struct{}{}
	}
	for name := range rt.constructors {
		names[name] = struct{}{}
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// PHPInfo prints a compact phpinfo-style text report for the phpscript
// runtime. It intentionally reports runtime facts rather than PHP extensions
// that phpscript does not provide.
func (rt *Runtime) PHPInfo() error {
	internal, user := rt.DefinedFunctions()
	_, err := fmt.Fprintf(rt.out, "phpscript\n\nRuntime => phpscript\nSAPI => %s\nGo Version => %s\nOperating System => %s\nArchitecture => %s\nInclude Path => %s\nWorking Directory => %s\nInternal Functions => %d\nUser Functions => %d\nDeclared Classes => %d\nDefined Constants => %d\n",
		rt.SAPI(), goruntime.Version(), goruntime.GOOS, goruntime.GOARCH,
		rt.IncludePath(), rt.WorkDir(), len(internal), len(user),
		len(rt.DeclaredClasses()), len(rt.constants))
	return err
}

// RegisterClass adds a resolved class to the class table so `new Name` works.
func (rt *Runtime) RegisterClass(c *model.Class) {
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
// true, registered autoloaders are given a chance to define a missing class.
func (rt *Runtime) ClassExists(name string, autoload bool) (bool, error) {
	name = strings.TrimPrefix(name, "\\")
	if rt.hasClass(name) {
		return true, nil
	}
	if !autoload {
		return false, nil
	}
	if err := rt.autoload(name, NewScope()); err != nil {
		return false, err
	}
	return rt.hasClass(name), nil
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
	return nil, false
}

func (rt *Runtime) lookupConstructor(name string) (any, bool) {
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

// OnError installs an error handler (register_error_handler). When set, runtime
// evaluation errors are routed here instead of aborting the caller.
func (rt *Runtime) OnError(fn func(error)) {
	rt.errorHandler = fn
}

// dynamicType is the sentinel value used in the compile-time type env to mark a
// variable as dynamically typed. expr-lang auto-dereferences the pointer and
// sees a bare interface{}, so it allows any operation on the value (PHP is
// dynamically typed). Without this, expr would infer concrete types and reject
// e.g. arithmetic across call sites where the type differs.
var dynamicType = new(any)

// Eval transpiles e, binds the referenced variables from scope, and runs the
// resulting program through the expr-lang VM.
func (rt *Runtime) Eval(e model.Expr, scope *Scope) (any, error) {
	if b, ok := e.(*model.Binary); ok && b.Op == "." {
		return rt.evalConcat(b, scope)
	}
	if u, ok := e.(*model.Unary); ok && (u.Op == "++" || u.Op == "--") {
		return rt.evalIncDec(u, scope)
	}
	if i, ok := e.(*model.Include); ok {
		return rt.evalInclude(i, scope)
	}

	ce, err := rt.compileExpr(e)
	if err != nil {
		return nil, err
	}

	base := rt.baseEnv(scope)
	defer rt.releaseEnv(base)

	// Anonymous functions become callables in the env (bound by their synthetic
	// identifier) so transpiled code can pass them, e.g. usort's comparator.
	for id, cl := range ce.closures {
		decl := cl
		base[id] = adapt(func(args ...any) (any, error) { return rt.invokeClosure(decl, args) })
	}
	base["__eval"] = adapt(rt.helperEval(scope, ce.exprs))

	// Run env: same functions/helpers, but variables carry their real values.
	// Bare identifiers that are not set in the current scope fall back to the
	// constant table (PHP constants are visible in every scope, whereas plain
	// variables are confined to their frame). This is what lets a method body
	// reference T_VARIABLE / a define()d constant the same way top-level code
	// can.
	for _, name := range ce.vars {
		if v, ok := scope.Get(name); ok {
			base[varIdent(name)] = v
		} else if c, ok := rt.constants[name]; ok {
			base[varIdent(name)] = c
		} else if _, ok := phpSuperglobals[name]; ok {
			base[varIdent(name)] = rt.globals[name]
		} else {
			base[varIdent(name)] = nil
		}
	}

	out, err := expr.Run(ce.prog, base)
	if err != nil {
		return nil, fmt.Errorf("eval %q: %w", ce.src, err)
	}
	return out, nil
}

func (rt *Runtime) compileExpr(e model.Expr) (*compiledExpr, error) {
	if rt.compiled != nil {
		if ce, ok := rt.compiled[e]; ok {
			return ce, nil
		}
	}

	tr := NewTranspiler()
	src, vars, err := tr.Transpile(e)
	if err != nil {
		return nil, err
	}
	closures := tr.Closures()
	exprs := tr.Exprs()
	if prog, ok := rt.exprCache.GetSource(src); ok {
		ce := &compiledExpr{src: src, vars: vars, closures: closures, exprs: exprs, prog: prog}
		rt.setCompiledExpr(e, ce)
		return ce, nil
	}

	base := rt.typeEnvBase()
	for id := range closures {
		base[id] = adapt(func(args ...any) (any, error) { return nil, nil })
	}
	if len(exprs) > 0 {
		base["__eval"] = adapt(func(args ...any) any { return nil })
	}
	typeEnv := make(map[string]any, len(base)+len(vars))
	maps.Copy(typeEnv, base)
	for _, name := range vars {
		typeEnv[varIdent(name)] = dynamicType
	}

	prog, err := rt.compile(src, typeEnv)
	if err != nil {
		return nil, fmt.Errorf("compile %q: %w", src, err)
	}
	ce := &compiledExpr{src: src, vars: vars, closures: closures, exprs: exprs, prog: prog}
	rt.exprCache.SetSource(src, prog)
	rt.setCompiledExpr(e, ce)
	return ce, nil
}

func (rt *Runtime) setCompiledExpr(e model.Expr, ce *compiledExpr) {
	if rt.compiled == nil {
		rt.compiled = make(map[model.Expr]*compiledExpr)
	}
	rt.compiled[e] = ce
}

// compile returns a cached compiled program for src.
//
// We deliberately compile without expr.Env type information. PHP is
// dynamically typed and the same expression may see different value types
// across invocations, so static type checking would be counter-productive here.
// Identifiers resolve from the runtime env map instead.
//
// All expr-lang builtins are disabled. PHP brings its own standard library via
// forwarded/registered functions (RegisterFunc), and expr's builtins (count,
// len, all, ...) would otherwise shadow PHP functions of the same name. With
// builtins off, a registered `count` resolves to the user's implementation.
func (rt *Runtime) compile(src string, typeEnv map[string]any) (*vm.Program, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.cache != nil {
		if p, ok := rt.cache[src]; ok {
			return p, nil
		}
	}
	p, err := expr.Compile(src, expr.Env(typeEnv), expr.DisableAllBuiltins())
	if err != nil {
		return nil, err
	}
	if rt.cache == nil {
		rt.cache = make(map[string]*vm.Program)
	}
	rt.cache[src] = p
	return p, nil
}

// baseEnv builds the env shared by every evaluation: the forwarded/registered
// functions plus the PHP-semantic helpers. Because expr-lang builtins are
// disabled (see compile), registered functions are exposed as bare identifiers
// and called directly by name. Variables are layered on top by Eval.
//
// Every function is wrapped with adapt() into a uniform func(...any) (any,
// error) signature. This lets the compile-time type env accept dynamically
// typed PHP arguments and keeps the runtime func type in sync with the type
// env (see helpers.go::adapt and Eval).
func (rt *Runtime) baseEnv(scope *Scope) map[string]any {
	env := evalEnvPool.Get().(map[string]any)
	for name, fn := range rt.funcs {
		callable := fn
		env[name] = func(args ...any) (any, error) {
			return rt.invokeWithScopeContext(callable, args, scope)
		}
	}
	// PHP-semantic helpers (see helpers.go). They close over rt and scope so
	// that method dispatch and instantiation can re-enter the interpreter.
	for name, fn := range rt.helpers {
		env[name] = fn
	}
	env["__call"] = adapt(rt.helperCall(scope))
	env["__get"] = adapt(rt.helperGet(scope))
	env["__new"] = adapt(rt.helperNew(scope))
	env["__classconst"] = adapt(rt.helperClassConst(scope))
	env["__set"] = adapt(rt.helperSet(scope))
	env["__ref"] = adapt(rt.helperRef(scope))
	env["__func"] = adapt(rt.helperFunc(scope))
	// func_get_args() needs the current frame's arguments, so it is provided as
	// a scope-aware helper rather than a plain forwarded function.
	env["func_get_args"] = adapt(func() *model.Array {
		arr := model.NewArray()
		if v, ok := scope.Get(argsKey); ok {
			if args, ok := v.([]any); ok {
				for _, a := range args {
					arr.Append(a)
				}
			}
		}
		return arr
	})
	return env
}

func (rt *Runtime) releaseEnv(env map[string]any) {
	oversized := len(env) > 256
	for k := range env {
		delete(env, k)
	}
	if oversized {
		return
	}
	evalEnvPool.Put(env)
}

func (rt *Runtime) typeEnvBase() map[string]any {
	env := make(map[string]any, len(rt.wrapped)+13)
	for name, fn := range rt.wrapped {
		env[name] = fn
	}
	stub := adapt(func(args ...any) any { return nil })
	env["__bool"] = stub
	env["__concat"] = stub
	env["__pair"] = stub
	env["__array"] = stub
	env["__index"] = stub
	env["__get"] = stub
	env["__call"] = stub
	env["__new"] = stub
	env["__cast"] = stub
	env["__arith"] = stub
	env["__classconst"] = stub
	env["__set"] = stub
	env["__ref"] = stub
	env["__func"] = stub
	env["func_get_args"] = stub
	return env
}

// helperSet implements assignment used as an expression (AssignExpr with a Var
// target): it mutates the current scope and returns the assigned value so the
// surrounding expression (e.g. a comparison) can use it.
func (rt *Runtime) helperSet(scope *Scope) func(name string, val any) any {
	return func(name string, val any) any {
		scope.Set(name, val)
		return val
	}
}

func (rt *Runtime) evalIncDec(n *model.Unary, scope *Scope) (any, error) {
	cur, err := rt.readLValue(n.X, scope)
	if err != nil {
		return nil, err
	}
	next := toInt(cur) + 1
	if n.Op == "--" {
		next = toInt(cur) - 1
	}
	if err := rt.assignTo(n.X, next, scope); err != nil {
		return nil, err
	}
	if n.Postfix {
		return cur, nil
	}
	return next, nil
}

func (rt *Runtime) evalConcat(n *model.Binary, scope *Scope) (any, error) {
	parts := flattenConcat(n, nil)
	var out strings.Builder
	for _, part := range parts {
		v, err := rt.Eval(part, scope)
		if err != nil {
			return nil, err
		}
		out.WriteString(phpString(v))
	}
	return out.String(), nil
}

func flattenConcat(e model.Expr, out []model.Expr) []model.Expr {
	if b, ok := e.(*model.Binary); ok && b.Op == "." {
		out = flattenConcat(b.Left, out)
		return flattenConcat(b.Right, out)
	}
	return append(out, e)
}

func (rt *Runtime) helperEval(scope *Scope, exprs map[string]model.Expr) func(string) (any, error) {
	return func(id string) (any, error) {
		e := exprs[id]
		if e == nil {
			return nil, fmt.Errorf("eval: unknown expression marker %s", id)
		}
		return rt.Eval(e, scope)
	}
}

// helperFunc dispatches a (possibly namespace-qualified) free-function call. It
// looks up name in the runtime function table and, if missing, the global
// fallback name — matching PHP's namespace resolution where an unqualified call
// falls back to the global function of the same short name.
func (rt *Runtime) helperFunc(scope *Scope) func(name, fallback string, args ...any) (any, error) {
	return func(name, fallback string, args ...any) (any, error) {
		if fn, ok := rt.lookupFunc(name); ok {
			return rt.invokeWithScopeContext(fn, args, scope)
		}
		if fallback != "" {
			if fn, ok := rt.lookupFunc(fallback); ok {
				return rt.invokeWithScopeContext(fn, args, scope)
			}
		}
		return nil, fmt.Errorf("call to undefined function %s()", name)
	}
}

func (rt *Runtime) lookupFunc(name string) (any, bool) {
	if fn, ok := rt.funcs[name]; ok {
		return fn, true
	}
	for functionName, fn := range rt.funcs {
		if strings.EqualFold(functionName, name) {
			return fn, true
		}
	}
	return nil, false
}

// helperRef yields a setter for a by-reference output parameter (e.g.
// preg_match_all's $matches). The shim calls it to write back into scope.
func (rt *Runtime) helperRef(scope *Scope) func(name string) func(any) {
	return func(name string) func(any) {
		return func(v any) { scope.Set(name, v) }
	}
}

// helperClassConst resolves Class::NAME (and self::NAME), evaluating the
// constant's expression once and caching it.
func (rt *Runtime) helperClassConst(scope *Scope) func(class, name string) (any, error) {
	return func(class, name string) (any, error) {
		if class == "self" || class == "static" {
			if c, ok := scope.Get("__class__"); ok {
				class, _ = c.(string)
			}
		}
		if !rt.hasClass(class) {
			if err := rt.autoload(class, scope); err != nil {
				return nil, err
			}
		}
		c, ok := rt.lookupClass(class)
		if !ok {
			return nil, fmt.Errorf("class constant %s::%s: unknown class", class, name)
		}
		class = c.Name
		if cached, ok := rt.classConsts[class]; ok {
			if v, ok := cached[name]; ok {
				return v, nil
			}
		}
		for _, k := range c.Consts {
			if k.Name != name {
				continue
			}
			v, err := rt.Eval(k.Default, scope)
			if err != nil {
				return nil, err
			}
			if rt.classConsts[class] == nil {
				rt.classConsts[class] = map[string]any{}
			}
			rt.classConsts[class][name] = v
			return v, nil
		}
		return nil, fmt.Errorf("undefined class constant %s::%s", class, name)
	}
}
