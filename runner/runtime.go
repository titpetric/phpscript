package runner

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/titpetric/phpscript/model"
)

// Runtime is the abstraction over the expr-lang VM. It owns everything the
// transpiled expressions need to run:
//
//   - the output sink (echo target: stdout/stderr for CLI, a buffer for HTTP),
//   - the symbol table of forwarded/registered functions (the "bring your own
//     stdlib" mechanism from the README — register_function),
//   - the class table (resolved ClassDecls), and
//   - an optional error handler (register_error_handler).
//
// A Runtime evaluates a model.Expr by transpiling it to expr source, assembling
// an env (helpers + functions + the current scope's variables) and running it
// through a cached *vm.Program. Statements (control flow, mutation) are driven
// by the interpreter in runner.go, which calls back into Eval for leaf values.
type Runtime struct {
	out     io.Writer
	funcs   map[string]any
	wrapped map[string]func(...any) (any, error)
	classes map[string]*model.Class

	// constructors maps a class name to a Go constructor function, so PHP's
	// `new Name` instantiates a native Go value rather than a model.Object. This
	// is the bridge for "bring your own type": a constructor like
	// func(ctx context.Context) (Storage, error) makes `$s = new Storage;`
	// behave like `s, err := NewStorage(ctx)` (the error surfaces as a throw).
	constructors map[string]any

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

	// fsys is the source root for include/require resolution; parse turns a
	// file's bytes into a program. Wiring an fs.FS (os.DirFS / embed.FS) keeps
	// the runner host-agnostic while letting callers provide a source tree.
	fsys         fs.FS
	parse        ParseFunc
	includeCache *IncludeCache

	// classConsts caches evaluated class constants (Class::NAME) per class.
	classConsts map[string]map[string]any

	mu        sync.Mutex
	cache     map[string]*vm.Program // expr source -> compiled program
	exprCache *ExprCache
	envPool   sync.Pool
	helpers   map[string]func(...any) (any, error)
}

// ParseFunc turns PHP source into a program. It is injected (rather than
// imported) so the runner keeps no dependency on the parser package.
type ParseFunc func(src string) (*model.Program, error)

type compiledExpr struct {
	src      string
	vars     []string
	closures map[string]*model.Closure
	prog     *vm.Program
}

// ExprCache stores compiled expression programs by AST expression node. Like
// IncludeCache, it assumes parsed ASTs are immutable after parsing.
type ExprCache struct {
	mu    sync.Mutex
	exprs map[model.Expr]*compiledExpr
}

func NewExprCache() *ExprCache {
	return &ExprCache{exprs: map[model.Expr]*compiledExpr{}}
}

func (c *ExprCache) Get(e model.Expr) (*compiledExpr, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ce, ok := c.exprs[e]
	return ce, ok
}

func (c *ExprCache) Set(e model.Expr, ce *compiledExpr) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.exprs[e] = ce
}

// IncludeCache stores parsed include/require programs by cleaned filesystem
// path. Parsed programs are treated as immutable by Runtime.Run/exec: hoisting
// copies declarations into runtime maps, while statement execution only reads
// the AST, so cached *model.Program values can be shared safely by callers that
// do not mutate ASTs themselves.
type IncludeCache struct {
	mu       sync.Mutex
	programs map[string]*model.Program
}

func NewIncludeCache() *IncludeCache {
	return &IncludeCache{programs: map[string]*model.Program{}}
}

func (c *IncludeCache) Get(path string) (*model.Program, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	prog, ok := c.programs[path]
	return prog, ok
}

func (c *IncludeCache) Set(path string, prog *model.Program) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.programs[path] = prog
}

// New returns a Runtime that writes echo output to w (defaults to os.Stdout).
func New(w io.Writer) *Runtime {
	if w == nil {
		w = os.Stdout
	}
	rt := &Runtime{
		out:          w,
		funcs:        map[string]any{},
		wrapped:      map[string]func(...any) (any, error){},
		classes:      map[string]*model.Class{},
		constructors: map[string]any{},
		ctx:          context.Background(),
		globals:      map[string]any{},
		constants:    map[string]any{},
		classConsts:  map[string]map[string]any{},
		cache:        map[string]*vm.Program{},
		exprCache:    NewExprCache(),
		helpers: map[string]func(...any) (any, error){
			"__bool":   adapt(phpTruthy),
			"__concat": adapt(helperConcat),
			"__pair":   adapt(helperPair),
			"__array":  adapt(helperArray),
			"__index":  adapt(helperIndex),
			"__get":    adapt(helperGet),
			"__cast":   adapt(helperCast),
		},
	}
	rt.envPool.New = func() any { return make(map[string]any, 128) }

	rt.RegisterConstructor("Exception", NewException)

	return rt
}

func NewException(message string) string {
	return message
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

// RegisterConstructor binds a class name to a Go constructor so `new Name` in
// PHP instantiates a native Go value. The constructor may take a leading
// context.Context (auto-injected) and may return a trailing error, which is
// surfaced to the interpreter as a thrown error. Example:
//
//	rt.RegisterConstructor("Storage", func(ctx context.Context) (Storage, error) { ... })
//	// PHP:  $storage = new Storage;   // == storage, err := NewStorage(ctx)
func (rt *Runtime) RegisterConstructor(name string, ctor any) {
	rt.constructors[name] = ctor
}

// SetFS installs the source root used to resolve include/require, with the
// parser used to compile each included file. This is the runner's "root dir"
// abstraction: pass os.DirFS(dir) for a directory tree or an embed.FS for an
// embedded one.
func (rt *Runtime) SetFS(fsys fs.FS, parse ParseFunc) {
	rt.fsys = fsys
	rt.parse = parse
	rt.includeCache = NewIncludeCache()
}

// SetIncludeCache installs a shared include cache. Passing nil disables include
// caching until SetFS is called again.
func (rt *Runtime) SetIncludeCache(cache *IncludeCache) { rt.includeCache = cache }

// SetExprCache installs a shared compiled-expression cache. Passing nil
// disables AST-node expression caching for this runtime.
func (rt *Runtime) SetExprCache(cache *ExprCache) { rt.exprCache = cache }

// FS returns the configured source root (or nil).
func (rt *Runtime) FS() fs.FS { return rt.fsys }

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

// RegisterClass adds a resolved class to the class table so `new Name` works.
func (rt *Runtime) RegisterClass(c *model.Class) {
	rt.classes[c.Name] = c
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
	if ce, ok := rt.exprCache.Get(e); ok {
		return ce, nil
	}

	tr := NewTranspiler()
	src, vars, err := tr.Transpile(e)
	if err != nil {
		return nil, err
	}

	base := rt.typeEnvBase()
	for id := range tr.Closures() {
		base[id] = adapt(func(args ...any) (any, error) { return nil, nil })
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
	ce := &compiledExpr{src: src, vars: vars, closures: tr.Closures(), prog: prog}

	if cached, ok := rt.exprCache.Get(e); ok {
		return cached, nil
	}
	rt.exprCache.Set(e, ce)
	return ce, nil
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
	if p, ok := rt.cache[src]; ok {
		return p, nil
	}
	p, err := expr.Compile(src, expr.Env(typeEnv), expr.DisableAllBuiltins())
	if err != nil {
		return nil, err
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
	env := rt.envPool.Get().(map[string]any)
	for name, fn := range rt.wrapped {
		env[name] = fn
	}
	// PHP-semantic helpers (see helpers.go). They close over rt and scope so
	// that method dispatch and instantiation can re-enter the interpreter.
	for name, fn := range rt.helpers {
		env[name] = fn
	}
	env["__call"] = adapt(rt.helperCall(scope))
	env["__new"] = adapt(rt.helperNew(scope))
	env["__classconst"] = adapt(rt.helperClassConst(scope))
	env["__set"] = adapt(rt.helperSet(scope))
	env["__ref"] = adapt(rt.helperRef(scope))
	env["__func"] = adapt(rt.helperFunc())
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
	for k := range env {
		delete(env, k)
	}
	rt.envPool.Put(env)
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

// helperFunc dispatches a (possibly namespace-qualified) free-function call. It
// looks up name in the runtime function table and, if missing, the global
// fallback name — matching PHP's namespace resolution where an unqualified call
// falls back to the global function of the same short name.
func (rt *Runtime) helperFunc() func(name, fallback string, args ...any) (any, error) {
	return func(name, fallback string, args ...any) (any, error) {
		if fn, ok := rt.funcs[name]; ok {
			return invokeAny(fn, args)
		}
		if fallback != "" {
			if fn, ok := rt.funcs[fallback]; ok {
				return invokeAny(fn, args)
			}
		}
		return nil, fmt.Errorf("call to undefined function %s()", name)
	}
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
		if cached, ok := rt.classConsts[class]; ok {
			if v, ok := cached[name]; ok {
				return v, nil
			}
		}
		c, ok := rt.classes[class]
		if !ok {
			return nil, fmt.Errorf("class constant %s::%s: unknown class", class, name)
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
