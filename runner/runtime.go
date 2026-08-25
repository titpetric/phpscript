package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"reflect"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/checker"
	"github.com/expr-lang/expr/checker/nature"
	"github.com/expr-lang/expr/compiler"
	"github.com/expr-lang/expr/conf"
	"github.com/expr-lang/expr/file"
	"github.com/expr-lang/expr/optimizer"
	"github.com/expr-lang/expr/vm"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/telemetry"
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
	out        io.Writer
	outStack   []io.Writer
	flat       bool
	status     telemetry.State
	entrypoint string
	observers  []Observer
	funcs      map[string]any
	userFns    map[string]struct{}
	classes    map[string]*model.Class

	// Env is the environment visible to PHP for this Runtime. New snapshots the
	// host environment so mutations remain local to a single request/runtime.
	Env map[string]string

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
	// context.Context, mirroring vuego's wrapContextFunc. It lets PHP call
	// these symbols without supplying the context argument explicitly.
	ctx context.Context

	errorHandler func(error)
	include      IncludeFunc
	includeHooks map[string]func() (any, error)
	globals      map[string]any
	shutdown     []any

	// constants holds PHP constants (define()/named T_* etc.). Unlike globals,
	// constants are visible in every scope, including inside functions and
	// methods, matching PHP's constant semantics. Bare-identifier lookups that
	// miss the current scope fall back here (see Eval).
	constants    map[string]any
	frozenConsts map[string]any
	constsShared bool

	// opts holds runtime source root, working directory, and write policy.
	opts         Options
	includeCache *IncludeCache
	included     []string

	// classConsts caches evaluated class constants (Class::NAME) per class.
	classConsts map[string]map[string]any

	// classStatics holds the live value of every `static $name` property, one
	// bag per class. Unlike a constant a static is mutable and shared, so the
	// bag is the storage itself rather than a cache: the declaration's default
	// seeds it the first time the class is touched, and every later read and
	// write of Class::$name goes through it.
	classStatics map[string]map[string]any

	mu    sync.Mutex
	cache map[string]*vm.Program // expr source -> compiled program
	// exprConf is the expr-lang compile configuration derived from the
	// compile-time type env. Deriving it is the expensive half of a compile
	// (expr walks the whole function table reflectively), so it is built once per
	// function-table generation and reused. Guarded by mu, which compile holds.
	exprConf    *conf.Config
	exprConfGen uint64
	exprCache   *ExprCache
	compiled    map[model.Expr]*compiledExpr
	helpers     map[string]func(...any) (any, error)

	// funcsGen is bumped whenever the function table changes (RegisterFunc, a
	// hoisted user function). Prebuilt evaluation environments and the cached
	// compile-time type env are stamped with the generation they were built
	// from and rebuilt when they fall behind.
	funcsGen uint64

	// envMu guards envFree, the free list of reusable evaluation environments.
	envMu      sync.Mutex
	envFree    []*evalEnv
	typeEnv    map[string]any
	typeEnvGen uint64

	sourceSpans map[model.Stmt]model.SourceSpan
	currentLine int

	// frames is the stack of live interpreter frames, global frame first;
	// vmWalkers enumerate the live values of any flat VM currently running.
	// Together with globals and classStatics they are the roots MemoryWalk
	// measures.
	frames    []*Scope
	vmWalkers []func(yield func(any))

	memBase    int64 // host request overhead accounted at the boundary (AccountRequest)
	memUsage   int64 // cached result of the last MemoryWalk
	memPeak    int64 // high-water mark, refreshed at every walk
	memTick    int   // statements since the last checkpoint walk
	memPending int64 // shallow bytes host calls produced since the last walk
}

// maxFreeEnvs bounds the per-Runtime free list of evaluation environments. Each
// entry retains one closure per registered function, so the list is kept small;
// nesting deeper than this simply builds (and discards) an environment the way
// every Eval used to.
const maxFreeEnvs = 16

// envSizeHint presizes an evaluation environment's map. It used to be the size
// of the whole function table, which is no longer what an environment holds:
// since installFunc adds registered functions on demand, an environment carries
// the ~16 PHP-semantic helpers, the functions its expressions actually call, and
// a handful of layered per-expression keys. The hint only has to avoid the first
// few rehashes; an environment that outgrows it grows once and is then reused.
const envSizeHint = 48

// scopeRef is the indirection that lets the environment's closures be built
// once and still see the scope of the evaluation currently using them. Helpers
// read ref.scope at call time; anything that outlives the evaluation (a bound
// method, a by-reference setter) copies the scope out of the ref first.
type scopeRef struct {
	scope *Scope
}

// evalEnv is one reusable expr environment: the registered functions and the
// PHP-semantic helpers, built once per function-table generation, plus the
// per-expression keys layered on top by Eval and removed again on release.
type evalEnv struct {
	ref     *scopeRef
	env     map[string]any
	exprs   map[string]model.Expr
	layered []string
	shadow  map[string]any
	gen     uint64
	built   bool
}

// layer installs a per-expression key, remembering it so release can remove it.
// A key that already exists in the prebuilt base (only possible if a registered
// function is named like a variable identifier) is restored rather than deleted.
func (st *evalEnv) layer(key string, val any) {
	if old, ok := st.env[key]; ok {
		if st.shadow == nil {
			st.shadow = map[string]any{}
		}
		st.shadow[key] = old
	}
	st.layered = append(st.layered, key)
	st.env[key] = val
}

// ExitError is returned when PHP die()/exit() interrupts script execution.
type ExitError struct {
	Code int
}

// Error returns a formatted PHP exit status.
func (e *ExitError) Error() string {
	return fmt.Sprintf("exit(%d)", e.Code)
}

// ScriptExit reports the status a script ended with, and marks this error as
// an ending rather than a failure.
//
// It exists so a package that cannot name *ExitError can still recognise one:
// runner imports flatstack/engine, so engine cannot import runner back, and
// the VM has to know an exit when it unwinds one past a catch clause. Asking
// the error what it is keeps that seam an interface rather than a string
// comparison on the message.
func (e *ExitError) ScriptExit() int {
	return e.Code
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

// Output returns the writer script output goes to: echo statements, inline
// HTML, and any builtin that emits text of its own, such as die() with a
// message, so their text joins the response body in the order the script
// produced it.
//
// While a redirection is active (PushOutput, which output buffering is built
// on) this is the innermost writer, so everything the script emits is captured
// rather than sent on.
func (rt *Runtime) Output() io.Writer {
	if n := len(rt.outStack); n > 0 {
		return rt.outStack[n-1]
	}
	return rt.out
}

// New returns a Runtime that writes echo output to w (defaults to os.Stdout).
func New(w io.Writer, opts Options) *Runtime {
	if w == nil {
		w = os.Stdout
	}
	if opts.Stdin == nil {
		opts.Stdin = strings.NewReader("")
	}
	opts.WorkDir = cleanFSPath(opts.WorkDir)
	rt := &Runtime{
		out:          w,
		Env:          ScriptEnvironment(opts.Env),
		opts:         opts,
		includeCache: NewIncludeCache(),
		funcs:        map[string]any{},
		userFns:      map[string]struct{}{},
		classes:      map[string]*model.Class{},
		constructors: map[string]any{},
		includePath:  ".",
		ctx:          context.Background(),
		globals:      map[string]any{},
		constants:    map[string]any{},
		classConsts:  map[string]map[string]any{},
		classStatics: map[string]map[string]any{},
		exprCache:    NewExprCache(),
		sourceSpans:  map[model.Stmt]model.SourceSpan{},
		helpers: map[string]func(...any) (any, error){
			"__bool":       adapt(phpTruthy),
			"__concat":     adapt(helperConcat),
			"__pair":       adapt(helperPair),
			"__array":      adapt(helperArray),
			"__index":      adapt(helperIndex),
			"__cast":       adapt(helperCast),
			"__arith":      adapt(phpArith),
			"__bit":        adapt(phpBitwise),
			"__bitnot":     adapt(phpBitNot),
			"__instanceof": adapt(phpInstanceOf),
			"__neg":        adapt(phpNegate),
		},
		memUsage: runtimeBaseline,
	}
	return rt
}

// InfrastructurePrefix names the variables that configure phpscript and the
// platform it runs on: connection strings, the listen address, the telemetry
// block. They are the host's configuration, not the script's environment, and a
// script never sees them.
//
// The rule already held for what a configuration file declares: a
// PLATFORM_DB_* entry registers a connection and is not added to PHP variables.
// It holds for the process environment for the same reason, and it matters more
// now that one process serves several sites: a tenant reading the operator's
// connection strings out of getenv() would make the per-site database boundary
// pointless.
const InfrastructurePrefix = "PLATFORM_"

// ScriptEnvironment returns the environment scripts read with getenv().
//
// A nil environment means the process environment, which is what a CLI run
// has. A host that configured one passes it instead, and a virtual host always
// does, so that a site sees the variables it declared rather than everything
// the operator's process happens to carry. Either way the infrastructure
// variables are held back.
func ScriptEnvironment(environment []string) map[string]string {
	if environment == nil {
		environment = os.Environ()
	}
	env := make(map[string]string, len(environment))
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		if name == "" || strings.HasPrefix(name, InfrastructurePrefix) {
			continue
		}
		env[name] = value
	}
	return env
}

// NewFlatStack returns a Runtime that executes supported programs through the
// flat bytecode backend and atomically falls back to the interpreter otherwise.
// Most callers should use flatstack.New, which preserves runner's public API.
func NewFlatStack(w io.Writer, opts Options) *Runtime {
	rt := New(w, opts)
	rt.flat = true
	return rt
}

// FreezeStdlib snapshots constants after host bindings are registered so
// ResetSession can drop script-defined constants without losing the stdlib.
func (rt *Runtime) FreezeStdlib() {
	rt.frozenConsts = rt.constants
	rt.constsShared = true
}

// ResetSession prepares the runtime to execute another program: new output and
// stdin, empty globals and user declarations. Host functions, constructors and
// the expression/bytecode caches stay.
func (rt *Runtime) ResetSession(out io.Writer, stdin io.Reader) {
	if out == nil {
		out = os.Stdout
	}
	rt.out = out
	rt.outStack = nil
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	rt.opts.Stdin = stdin
	clear(rt.userFns)
	clear(rt.classes)
	clear(rt.globals)
	rt.shutdown = nil
	rt.autoloaders = nil
	clear(rt.classConsts)
	clear(rt.classStatics)
	rt.entrypoint = ""
	rt.memBase = 0
	rt.memUsage = 0
	rt.memPeak = 0
	rt.memTick = 0
	rt.memPending = 0
	if rt.frozenConsts != nil {
		rt.constants = rt.frozenConsts
		rt.constsShared = true
	}
}

// SetContext installs the lifecycle context auto-injected into registered Go
// callables whose first parameter is a context.Context (constructors, methods,
// functions). Defaults to context.Background().
func (rt *Runtime) SetContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	rt.ctx = ctx
	for _, observer := range rt.observers {
		observer.UpdateStatus(rt.ctx, rt.status)
	}
}

// Context returns the configured lifecycle context.
func (rt *Runtime) Context() context.Context {
	return rt.ctx
}

// Observer receives lifecycle updates for a Runtime. Implementations must be
// safe for use by concurrent runtimes. A span is returned so the interpreter
// can measure the region it just reported; a nil span is valid and every
// method on it does nothing, which is what an observer with no trace in the
// context returns.
type Observer interface {
	UpdateStatus(context.Context, telemetry.State)
	Trace(context.Context, string, ...telemetry.Kind) *telemetry.Span
}

// FilenameObserver optionally receives the entrypoint passed to LoadFile.
type FilenameObserver interface {
	UpdateFilename(context.Context, string)
}

// IncludeObserver optionally receives the number of files included so far.
type IncludeObserver interface {
	UpdateIncludedFiles(context.Context, int)
}

// Observe registers an observer and reports that this Runtime is starting.
func (rt *Runtime) Observe(observer Observer) {
	if observer == nil {
		return
	}
	rt.observers = append(rt.observers, observer)
	if rt.status == "" {
		rt.status = telemetry.StateStarting
	}
	observer.UpdateStatus(rt.ctx, rt.status)
}

// UpdateStatus publishes a lifecycle phase to all registered observers. It is
// also available to hosts for phases that occur outside PHP execution.
func (rt *Runtime) UpdateStatus(state telemetry.State) {
	if rt.status == state {
		return
	}
	rt.status = state
	for _, observer := range rt.observers {
		observer.UpdateStatus(rt.ctx, state)
	}
}

// MemoryUsage returns the current request-scoped memory estimation in bytes,
// computed by a fresh walk of the live roots.
func (rt *Runtime) MemoryUsage() int64 {
	return rt.MemoryWalk()
}

// MemoryPeak returns the high-water usage mark. Peak is sampled at walk
// points (memory_get_usage calls and limit checkpoints), so an allocation
// both made and released between walks does not raise it, unlike PHP's
// allocator-level peak.
func (rt *Runtime) MemoryPeak() int64 {
	rt.MemoryWalk()
	return rt.memPeak
}

// MemoryLimit returns the configured memory limit.
func (rt *Runtime) MemoryLimit() Size {
	return rt.opts.MemoryLimit
}

// AccountRequest folds the size of host-owned request-lifetime values into
// the baseline the memory walk starts from: the request Context, the parsed
// *http.Request, the response writer. It is called once per value at the
// point a request crosses into the runtime; the walk then adds live script
// values on top. It is an estimate of what the request costs before any PHP
// evaluates, not an audit of every host allocation.
func (rt *Runtime) AccountRequest(values ...any) {
	visited := make(visitedSet)
	for _, v := range values {
		rt.memBase += DeepSize(v, visited)
	}
}

// MemoryWalk recomputes live usage from the roots: the interpreter frame
// stack, globals, class statics, and any running flat VM's live values.
// A visited set keyed on container identity counts a value reachable through
// several variables once and terminates cycles. The result refreshes the
// cached usage and the peak.
func (rt *Runtime) MemoryWalk() int64 {
	visited := make(visitedSet)
	total := runtimeBaseline + rt.memBase
	for _, scope := range rt.frames {
		for name, val := range scope.vars {
			// Double-underscore slots are interpreter bookkeeping, not PHP
			// variables (the DefinedVars rule).
			if len(name) >= 2 && name[:2] == "__" {
				continue
			}
			total += 16 + int64(len(name)) + DeepSize(val, visited)
		}
	}
	for _, val := range rt.globals {
		total += DeepSize(val, visited)
	}
	for _, bag := range rt.classStatics {
		for name, val := range bag {
			total += 16 + int64(len(name)) + DeepSize(val, visited)
		}
	}
	for _, walk := range rt.vmWalkers {
		walk(func(v any) {
			total += DeepSize(v, visited)
		})
	}
	rt.memUsage = total
	rt.memPending = 0
	if total > rt.memPeak {
		rt.memPeak = total
	}
	return total
}

// lastMemoryUsage returns the cached result of the last walk. Trace spans
// record this rather than pay for a walk apiece.
func (rt *Runtime) lastMemoryUsage() int64 {
	if rt.memUsage < runtimeBaseline {
		return runtimeBaseline
	}
	return rt.memUsage
}

// checkMemory walks and reports a RuntimeException when the configured
// MemoryLimit is exceeded. Without a limit it is a no-op.
func (rt *Runtime) checkMemory() error {
	if rt.opts.MemoryLimit <= 0 {
		return nil
	}
	if usage := rt.MemoryWalk(); rt.opts.MemoryLimit.Exceeds(usage) {
		return NewRuntimeException(fmt.Sprintf("Allowed memory size of %d bytes exhausted (%d bytes in use)", rt.opts.MemoryLimit.Bytes(), usage), 0)
	}
	return nil
}

func (rt *Runtime) pushFrame(s *Scope) {
	rt.frames = append(rt.frames, s)
}

func (rt *Runtime) popFrame() {
	rt.frames = rt.frames[:len(rt.frames)-1]
}

// newScope creates a fresh Scope.
func (rt *Runtime) newScope() *Scope {
	return NewScope()
}

// Trace publishes a trace span to registered observers and returns the first
// mutable span provided by one of them.
func (rt *Runtime) Trace(message string, kind ...telemetry.Kind) *telemetry.Span {
	return rt.traceContext(rt.ctx, message, kind...)
}

func (rt *Runtime) traceContext(ctx context.Context, message string, kind ...telemetry.Kind) *telemetry.Span {
	var span *telemetry.Span
	for _, observer := range rt.observers {
		observed := observer.Trace(ctx, message, kind...)
		if span == nil {
			span = observed
		}
	}
	if span != nil {
		span.SetAttribute("memory_usage", rt.lastMemoryUsage())
	}
	return span
}

// UpdateFilename publishes the PHP entrypoint to observers that support
// filename updates.
func (rt *Runtime) UpdateFilename(filename string) {
	if rt.entrypoint != "" {
		return
	}
	rt.entrypoint = filename
	rt.SetConst("__FILE__", filename)
	rt.SetConst("__DIR__", path.Dir(filename))
	for _, observer := range rt.observers {
		if filenameObserver, ok := observer.(FilenameObserver); ok {
			filenameObserver.UpdateFilename(rt.ctx, filename)
		}
	}
}

// UpdateIncludedFiles publishes the current number of included files.
func (rt *Runtime) UpdateIncludedFiles(count int) {
	for _, observer := range rt.observers {
		if includeObserver, ok := observer.(IncludeObserver); ok {
			includeObserver.UpdateIncludedFiles(rt.ctx, count)
		}
	}
}

// RegisterConstructor binds a class name to a Go constructor so `new Name` in
// PHP instantiates a native Go value. The constructor may take a leading
// context.Context (auto-injected) and may return a trailing error, which is
// surfaced to the interpreter as a thrown error. When a direct variable
// assignment receives a constructed value that implements SetID(string), the
// runtime passes it the PHP variable name without the leading dollar sign.
// Example:
//
//	rt.RegisterConstructor("Storage", func(ctx context.Context) (Storage, error) { ... }).
//	// PHP:  $storage = new Storage;   // == storage, err := NewStorage(ctx).
func (rt *Runtime) RegisterConstructor(name string, ctor any) {
	rt.constructors[name] = ctor
}

// LookupConstructor returns the host constructor registered for name.
func (rt *Runtime) LookupConstructor(name string) (any, bool) {
	fn, ok := rt.constructors[name]
	return fn, ok
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

// Stdin returns the input stream configured by the runtime host.
func (rt *Runtime) Stdin() io.Reader { return rt.opts.Stdin }

// WorkDir returns the configured working directory inside the source root.
func (rt *Runtime) WorkDir() string { return rt.opts.WorkDir }

// Database returns the provider named connections resolve through, or nil when
// the host configured none. The binding decides what nil falls back to.
func (rt *Runtime) Database() model.DatabaseProvider { return rt.opts.Database }

// WritablePaths returns the configured writable path whitelist.
func (rt *Runtime) WritablePaths() []string { return append([]string(nil), rt.opts.WritablePaths...) }

// UploadFileMode returns the mode move_uploaded_file() gives a stored upload,
// which is DefaultUploadFileMode unless the host configured one.
func (rt *Runtime) UploadFileMode() FileMode {
	if rt.opts.UploadFileMode == 0 {
		return DefaultUploadFileMode
	}
	return rt.opts.UploadFileMode
}

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
// and methods, unlike globals, which PHP confines to the global scope.
func (rt *Runtime) SetConst(name string, val any) {
	if rt.constsShared && rt.frozenConsts != nil {
		rt.constants = maps.Clone(rt.frozenConsts)
		rt.constsShared = false
	}
	rt.constants[name] = val
}

// RegisterShutdown appends a callback to run when the current program exits.
// Shutdown callbacks run in registration order, including after exit or error.
func (rt *Runtime) RegisterShutdown(callback any) {
	rt.shutdown = append(rt.shutdown, callback)
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
	// Prebuilt evaluation environments hold one closure per registered function;
	// bumping the generation makes them rebuild before their next use.
	rt.envMu.Lock()
	rt.funcsGen++
	rt.envMu.Unlock()
}

// LookupFunc returns the host function registered for name. Introspection
// tooling uses it to reflect over a binding's Go signature; a PHP user-defined
// function resolves too, as whatever callable the runtime stored for it.
func (rt *Runtime) LookupFunc(name string) (any, bool) {
	fn, ok := rt.funcs[name]
	return fn, ok
}

func (rt *Runtime) registerUserFunc(name string, fn any) {
	rt.RegisterFunc(name, fn)
	rt.userFns[name] = struct{}{}
}

// FunctionExists reports whether name resolves to a host shim or a PHP
// user-defined function, backing `function_exists`. Template engines guard
// their generated block functions with it, so an always-false answer would
// redeclare them on every include.
func (rt *Runtime) FunctionExists(name string) bool {
	_, ok := rt.lookupFunc(strings.TrimPrefix(name, "\\"))
	return ok
}

// IncludeFile evaluates path as PHP in a fresh scope, the same way an `include`
// statement would. Hosts that resolve classes outside the interpreter (the
// composer autoloader) use it to pull a declaration file into the runtime.
func (rt *Runtime) IncludeFile(path string) (any, error) {
	return rt.includeFile(path, rt.newScope())
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
	_, err := fmt.Fprintf(rt.Output(), "phpscript\n\nRuntime => phpscript\nSAPI => %s\nGo Version => %s\nOperating System => %s\nArchitecture => %s\nInclude Path => %s\nWorking Directory => %s\nInternal Functions => %d\nUser Functions => %d\nDeclared Classes => %d\nDefined Constants => %d\n",
		rt.SAPI(), goruntime.Version(), goruntime.GOOS, goruntime.GOARCH,
		rt.IncludePath(), rt.WorkDir(), len(internal), len(user),
		len(rt.DeclaredClasses()), len(rt.constants))
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
// true, registered autoloaders are given a chance to define a missing class.
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
// classes; a host-backed one is reflected over by its caller.
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

// Eval transpiles e, binds the referenced variables from scope, and runs the
// resulting program through the expr-lang VM.
func (rt *Runtime) Eval(e model.Expr, scope *Scope) (any, error) {
	if b, ok := e.(*model.Binary); ok && b.Op == "." {
		return rt.evalConcat(b, scope)
	}
	if s, ok := e.(*model.Interp); ok {
		return rt.joinParts(s.Parts, scope)
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

	st := rt.acquireEnv(scope)
	st.exprs = ce.exprs
	base := st.env
	defer rt.releaseEnv(st)

	// Anonymous functions become callables in the env (bound by their synthetic
	// identifier) so transpiled code can pass them, e.g. usort's comparator. They
	// capture the scope directly rather than reading it through st: a closure
	// assigned to a variable outlives this evaluation.
	//
	// They are layered before the calls below: an immediately invoked closure
	// appears in ce.calls under its synthetic identifier, which is in no
	// function table, and installFunc would stub it as undefined.
	for id, cl := range ce.closures {
		decl := cl
		env := captureClosureEnv(decl, scope)
		st.layer(id, adapt(func(args ...any) (any, error) {
			return rt.invokeClosure(decl, args, env)
		}))
	}

	// Registered functions are installed on demand: an environment carries the
	// PHP-semantic helpers plus whatever the expressions evaluated with it have
	// called so far, rather than a closure per entry of the function table. The
	// installed closures persist across evaluations (see installFunc).
	for _, name := range ce.calls {
		rt.installFunc(st, name)
	}

	// Run env: same functions/helpers, but variables carry their real values.
	// Bare identifiers that are not set in the current scope fall back to the
	// constant table (PHP constants are visible in every scope, whereas plain
	// variables are confined to their frame). This is what lets a method body
	// reference T_VARIABLE / a define()d constant the same way top-level code
	// can.
	for i, name := range ce.vars {
		if v, ok := scope.Get(name); ok {
			st.layer(ce.idents[i], v)
		} else if c, ok := rt.constants[name]; ok {
			st.layer(ce.idents[i], c)
		} else if _, ok := phpSuperglobals[name]; ok {
			st.layer(ce.idents[i], rt.globals[name])
		} else {
			st.layer(ce.idents[i], nil)
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

	// The transpiler is pooled; newCompiledExpr copies the variable slices it
	// hands out, so nothing survives the release.
	tr := acquireTranspiler()
	defer releaseTranspiler(tr)

	src, vars, err := tr.Transpile(e)
	if err != nil {
		return nil, err
	}
	idents := tr.Idents()
	calls := tr.Calls()
	closures := tr.Closures()
	exprs := tr.Exprs()
	if prog, ok := rt.exprCache.GetSource(src); ok {
		ce := newCompiledExpr(src, vars, idents, calls, closures, exprs, prog)
		rt.setCompiledExpr(e, ce)
		return ce, nil
	}

	prog, err := rt.compile(src)
	if err != nil {
		return nil, fmt.Errorf("compile %q: %w", src, err)
	}
	ce := newCompiledExpr(src, vars, idents, calls, closures, exprs, prog)
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
// Expression-local identifiers (PHP variables, closure bindings) are
// deliberately absent from the compile-time type env: PHP is dynamically typed
// and the same expression may see different value types across invocations, so
// static type checking of them would be counter-productive. They are compiled
// as undefined variables (conf.Strict is off, the equivalent of
// expr.AllowUndefinedVariables) and resolve from the runtime env map instead.
//
// The function table *is* part of the type env, and must be: expr's parser
// consults it (conf.Config.IsOverridden) to decide that names shared with
// expr's own predicate builtins (`count`, `map`, `filter`, `find`, `sum` and
// the rest) are user functions rather than builtin predicate syntax. Disabling the
// builtins is not enough on its own; predicates are parsed before the disabled
// list is consulted.
//
// All expr-lang builtins are disabled. PHP brings its own standard library via
// forwarded/registered functions (RegisterFunc), and expr's builtins (count,
// len, all, ...) would otherwise shadow PHP functions of the same name. With
// builtins off, a registered `count` resolves to the user's implementation.
func (rt *Runtime) compile(src string) (*vm.Program, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.cache != nil {
		if p, ok := rt.cache[src]; ok {
			return p, nil
		}
	}
	p, err := compileWith(src, rt.exprConfig())
	if err != nil {
		return nil, err
	}
	if rt.cache == nil {
		rt.cache = make(map[string]*vm.Program)
	}
	rt.cache[src] = p
	return p, nil
}

// exprConfig returns the expr compile configuration for the current function
// table, building it on first use and whenever the table changes.
//
// This is what expr.Compile(expr.Env(...)) does per call, and the reason it is
// hoisted: conf.Config.WithEnv walks the ~100-entry function table reflectively
// (MapKeys, MapIndex, a nature per entry) and used to dominate the interpreter's
// allocation profile. Nothing in the parse/check/optimize/compile pipeline
// writes to the Config, so one instance is reusable; the nature cache it carries
// is filled as a side effect and is guarded by rt.mu, which compile holds.
//
// Callers must hold rt.mu.
func (rt *Runtime) exprConfig() *conf.Config {
	rt.envMu.Lock()
	gen := rt.funcsGen
	rt.envMu.Unlock()
	if rt.exprConf != nil && rt.exprConfGen == gen {
		return rt.exprConf
	}

	env := rt.typeEnvBase()
	c := conf.CreateNew()
	c.EnvObject = env
	c.Env = typeEnvNature(&c.NtCache, env)
	// expr.AllowUndefinedVariables: PHP variables are not in the type env.
	c.Strict = false
	// expr.DisableAllBuiltins, plus the pruning expr.Compile does afterwards.
	for name := range c.Builtins {
		c.Disabled[name] = true
		delete(c.Builtins, name)
	}
	c.Check()

	rt.exprConf, rt.exprConfGen = c, gen
	return c
}

// compileWith runs expr's parse/check/optimize/compile pipeline against a
// prebuilt config. It mirrors expr.Compile, which cannot be used here because it
// insists on constructing a fresh conf.Config (and re-deriving the type env)
// on every call.
func compileWith(src string, c *conf.Config) (*vm.Program, error) {
	tree, err := checker.ParseCheck(src, c)
	if err != nil {
		return nil, err
	}
	if c.Optimize {
		if err := optimizer.Optimize(&tree.Node, c); err != nil {
			var fileError *file.Error
			if errors.As(err, &fileError) {
				return nil, fileError.Bind(tree.Source)
			}
			return nil, err
		}
	}
	return compiler.Compile(tree, c)
}

// acquireEnv returns an evaluation environment bound to scope. Environments are
// reused across evaluations: the expensive part, one closure per registered
// function plus the PHP-semantic helpers, is built once per function-table
// generation and thereafter only has its scope rebound.
func (rt *Runtime) acquireEnv(scope *Scope) *evalEnv {
	rt.envMu.Lock()
	var st *evalEnv
	if n := len(rt.envFree); n > 0 {
		st = rt.envFree[n-1]
		rt.envFree[n-1] = nil
		rt.envFree = rt.envFree[:n-1]
	}
	gen := rt.funcsGen
	rt.envMu.Unlock()

	if st == nil {
		st = &evalEnv{ref: &scopeRef{}, env: make(map[string]any, envSizeHint)}
	}
	if !st.built || st.gen != gen {
		rt.buildEnv(st, gen)
	}
	st.ref.scope = scope
	return st
}

// buildEnv fills st with the env shared by every evaluation: the PHP-semantic
// helpers. Because expr-lang builtins are disabled (see compile), registered
// functions are exposed as bare identifiers and called directly by name, but
// they are *not* installed here: installFunc adds the ones an expression
// actually calls, on demand (see Eval). Variables and other per-expression keys
// are layered on top by Eval.
//
// Every function is wrapped with adapt() into a uniform func(...any) (any,
// error) signature. This lets the compile-time type env accept dynamically
// typed PHP arguments and keeps the runtime func type in sync with the type
// env (see helpers.go::adapt and Eval).
//
// The closures read the scope through st.ref rather than capturing it, which is
// what makes the environment reusable.
func (rt *Runtime) buildEnv(st *evalEnv, gen uint64) {
	env := st.env
	clear(env)
	st.layered = st.layered[:0]
	clear(st.shadow)
	ref := st.ref
	// PHP-semantic helpers (see helpers.go). They close over rt and the scope
	// reference so that method dispatch and instantiation can re-enter the
	// interpreter.
	for name, fn := range rt.helpers {
		env[name] = fn
	}
	env["__call"] = adapt(rt.helperCall(ref))
	env["__get"] = adapt(rt.helperGet(ref))
	env["__new"] = adapt(rt.helperNew(ref))
	env["__classconst"] = adapt(rt.helperClassConst(ref))
	env["__set"] = adapt(rt.helperSet(ref))
	env["__ref"] = adapt(rt.helperRef(ref))
	env["__func"] = adapt(rt.helperFunc(ref))
	env["__static"] = adapt(rt.helperStaticCall(ref))
	env["__staticprop"] = adapt(rt.helperStaticProp(ref))
	env["__invoke"] = adapt(rt.helperInvoke(ref))
	// Expression markers (`__eval`) resolve against the expression currently
	// evaluated with this environment, which Eval stores on st.
	env["__eval"] = adapt(func(id string) (any, error) {
		e := st.exprs[id]
		if e == nil {
			return nil, fmt.Errorf("eval: unknown expression marker %s", id)
		}
		return rt.Eval(e, ref.scope)
	})
	// func_get_args() needs the current frame's arguments, so it is provided as
	// a scope-aware helper rather than a plain forwarded function.
	// The frame already holds its arguments as a []any, which the VM indexes and
	// iterates directly, so func_get_args() hands that slice back rather than
	// rebuilding it as an *model.Array.
	env["func_get_args"] = adapt(func() []any { return funcGetArgs(ref.scope) })
	st.gen = gen
	st.built = true
}

// installFunc adds the registered function name to st's environment, if it is
// not already there. The closure is left in the environment (it is not one of
// the per-expression keys releaseEnv strips), so a function is wrapped at most
// once per environment per function-table generation: buildEnv clears the map
// whenever the generation moves, which is exactly when a name could have been
// re-registered with a different implementation.
//
// A name that is in no function table installs a stub that reports the call as
// undefined, so this engine and flatstack (see helperFunc) give a script the
// same message instead of the VM's "cannot call nil". The stub is bound to the
// current generation like any other entry: declaring the function afterwards
// re-registers it, which moves the generation and rebuilds the environment.
//
// Names resolve case-insensitively, as PHP function names do and as helperFunc
// resolves them.
//
// Like buildEnv, the closure reads the scope through st.ref at call time rather
// than capturing it.
func (rt *Runtime) installFunc(st *evalEnv, name string) {
	if _, ok := st.env[name]; ok {
		return
	}
	ref := st.ref
	fn, ok := rt.lookupFunc(name)
	if !ok {
		st.env[name] = func(...any) (any, error) {
			return nil, fmt.Errorf("call to undefined function %s()", name)
		}
		return
	}
	st.env[name] = func(args ...any) (any, error) {
		result, err := rt.invokeWithScopeContext(fn, args, ref.scope)
		return result, nameCallError(err, name)
	}
}

// releaseEnv strips the per-expression keys layered on by Eval, drops the
// references the environment held to the scope and expression, and returns it
// to the free list.
func (rt *Runtime) releaseEnv(st *evalEnv) {
	if len(st.shadow) == 0 {
		for _, key := range st.layered {
			delete(st.env, key)
		}
	} else {
		for _, key := range st.layered {
			if old, ok := st.shadow[key]; ok {
				st.env[key] = old
				delete(st.shadow, key)
				continue
			}
			delete(st.env, key)
		}
	}
	st.layered = st.layered[:0]
	st.exprs = nil
	st.ref.scope = nil

	rt.envMu.Lock()
	if len(rt.envFree) < maxFreeEnvs {
		rt.envFree = append(rt.envFree, st)
	}
	rt.envMu.Unlock()
}

// typeEnvStub is the value every entry of the compile-time type env holds. The
// type env carries types, never values (expr derives one "nature" per entry and
// never calls it), and every callable the runtime exposes has been through
// adapt(), so they all share this one signature. A single shared stub therefore
// describes the whole function table exactly as well as a per-function wrapper
// would, and costs one closure instead of one per registered function.
var typeEnvStub any = func(...any) (any, error) { return nil, nil }

// typeEnvMapType is the reflect type of the compile-time type env map.
var typeEnvMapType = reflect.TypeOf(map[string]any(nil))

// typeEnvNature builds the conf.Config.Env nature for the compile-time type env
// without expr's reflective walk over it.
//
// conf.Config.WithEnv -> conf.EnvWithCache derives one nature per entry through
// reflect.Value.MapKeys + MapIndex + copyVal + a fresh nature.TypeData, which for
// a ~100-entry function table was the single largest allocation site left in the
// interpreter (12% of all objects). It exists because a general env map holds
// values of many different types.
//
// This one does not: every entry is typeEnvStub (see above), so every entry's
// nature is the nature of that one func type. Deriving it once and storing the
// same value under every key produces a nature that is equal to the one expr
// builds; TestCompileMatchesExprEnv pins that by comparing emitted bytecode.
//
// The shared nature.TypeData that all the entries then point at is written to
// only by nature's own lazy memoisation (NumIn, NumOut, Out, IsVariadic, the
// method set), all of which are functions of the type alone and therefore
// identical for every entry. The one field that carries per-name state,
// TypeData.Func, is set by the checker only for conf.Config.Functions and
// Builtins, both empty here, never for a nature that came out of the env.
func typeEnvNature(cache *nature.Cache, env map[string]any) nature.Nature {
	n := cache.FromType(typeEnvMapType)
	if n.TypeData == nil {
		n.TypeData = new(nature.TypeData)
	}
	n.Strict = true
	n.Fields = make(map[string]nature.Nature, len(env))
	stub := cache.NatureOf(typeEnvStub)
	for name := range env {
		n.Fields[name] = stub
	}
	return n
}

// typeEnvBase returns the compile-time type env for the current function table:
// every registered function plus the PHP-semantic helpers, each mapped to
// typeEnvStub.
//
// It exists so expr knows which names are functions. Two things depend on that:
// the parser, which must not read `count(...)` as its own builtin predicate
// (conf.Config.IsOverridden), and the checker, which resolves a call on a known
// name to (any, error). The result is cached per function-table generation and
// must be treated as read-only by callers.
func (rt *Runtime) typeEnvBase() map[string]any {
	rt.envMu.Lock()
	cached, gen, current := rt.typeEnv, rt.typeEnvGen, rt.funcsGen
	rt.envMu.Unlock()
	if cached != nil && gen == current {
		return cached
	}

	env := make(map[string]any, len(rt.funcs)+16)
	for name := range rt.funcs {
		env[name] = typeEnvStub
	}
	env["__bool"] = typeEnvStub
	env["__concat"] = typeEnvStub
	env["__pair"] = typeEnvStub
	env["__array"] = typeEnvStub
	env["__index"] = typeEnvStub
	env["__get"] = typeEnvStub
	env["__call"] = typeEnvStub
	env["__new"] = typeEnvStub
	env["__cast"] = typeEnvStub
	env["__arith"] = typeEnvStub
	env["__bit"] = typeEnvStub
	env["__bitnot"] = typeEnvStub
	env["__instanceof"] = typeEnvStub
	env["__neg"] = typeEnvStub
	env["__classconst"] = typeEnvStub
	env["__set"] = typeEnvStub
	env["__ref"] = typeEnvStub
	env["__func"] = typeEnvStub
	env["__static"] = typeEnvStub
	env["__staticprop"] = typeEnvStub
	env["__invoke"] = typeEnvStub
	env["__eval"] = typeEnvStub
	env["func_get_args"] = typeEnvStub

	rt.envMu.Lock()
	rt.typeEnv, rt.typeEnvGen = env, current
	rt.envMu.Unlock()
	return env
}

// helperSet implements assignment used as an expression (AssignExpr with a Var
// target): it mutates the current scope and returns the assigned value so the
// surrounding expression (e.g. a comparison) can use it.
func (rt *Runtime) helperSet(ref *scopeRef) func(name string, val any) any {
	return func(name string, val any) any {
		ref.scope.Set(name, val)
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
	return rt.joinParts(flattenConcat(n, nil), scope)
}

// joinParts evaluates each part and joins their PHP string forms. It is what a
// concatenation and an interpolated literal both reduce to, so the two produce
// the same value for the same operands, and neither pays for a trip through the
// expression VM to find that out.
func (rt *Runtime) joinParts(parts []model.Expr, scope *Scope) (any, error) {
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

// helperFunc dispatches a (possibly namespace-qualified) free-function call. It
// looks up name in the runtime function table and, if missing, the global
// fallback name, matching PHP's namespace resolution where an unqualified call
// falls back to the global function of the same short name.
func (rt *Runtime) helperFunc(ref *scopeRef) func(name, fallback string, args ...any) (any, error) {
	return func(name, fallback string, args ...any) (any, error) {
		scope := ref.scope
		if fn, ok := rt.lookupFunc(name); ok {
			result, err := rt.invokeWithScopeContext(fn, args, scope)
			return result, nameCallError(err, name)
		}
		if fallback != "" {
			if fn, ok := rt.lookupFunc(fallback); ok {
				result, err := rt.invokeWithScopeContext(fn, args, scope)
				return result, nameCallError(err, fallback)
			}
			// Frame-aware builtins live in the evaluation environment rather
			// than the function table, so the bare-name fast path finds them
			// but this dispatch would not. Inside a namespaced file every
			// global call arrives here, so they are resolved explicitly.
			if v, ok := scopeBuiltin(fallback, scope); ok {
				return v, nil
			}
		}
		if v, ok := scopeBuiltin(name, scope); ok {
			return v, nil
		}
		return nil, fmt.Errorf("call to undefined function %s()", name)
	}
}

// scopeBuiltin resolves the builtins provided per-environment instead of
// through the function table, under the name a PHP script calls them by.
func scopeBuiltin(name string, scope *Scope) (any, bool) {
	if strings.EqualFold(name, "func_get_args") {
		return funcGetArgs(scope), true
	}
	return nil, false
}

// funcGetArgs returns the arguments of the frame scope belongs to.
func funcGetArgs(scope *Scope) []any {
	if v, ok := scope.Get(argsKey); ok {
		if args, ok := v.([]any); ok {
			return args
		}
	}
	return nil
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
func (rt *Runtime) helperRef(ref *scopeRef) func(name string) func(any) {
	return func(name string) func(any) {
		// The setter is handed to a shim and may be called after this expression
		// finishes, so it binds the scope value rather than the reference.
		scope := ref.scope
		return func(v any) { scope.Set(name, v) }
	}
}

// helperClassConst resolves Class::NAME (and self::NAME), evaluating the
// constant's expression once and caching it.
func (rt *Runtime) helperClassConst(ref *scopeRef) func(class, name string) (any, error) {
	return func(class, name string) (any, error) {
		scope := ref.scope
		class = resolveClassName(class, scope)
		// `Class::class` is the class name itself. PHP resolves it at compile
		// time, so it does not require the class to be declared, and composer's
		// generated autoloader relies on that.
		if name == "class" {
			return class, nil
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
