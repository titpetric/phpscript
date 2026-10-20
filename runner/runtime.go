package runner

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/runner/expr"
	"github.com/titpetric/phpscript/telemetry"
)

// setVar routes a whole-variable assignment. A superglobal is one binding per
// request, visible in every scope, so writing the variable itself rebinds the
// global, the way PHP lets a script replace $_POST wholesale, and clears any
// scope-local shadow so reads keep resolving through the global. Every other
// name belongs to the scope. The superglobal set lives in phpval.AutoGlobals,
// whose claim check is a byte compare and no map probe: this runs on
// every assignment.
func (rt *Runtime) setVar(scope *Scope, name string, val any) {
	if rt.auto.Set(name, val) {
		scope.Unset(name)
		return
	}
	scope.Set(name, val)
}

// Runtime executes parsed PHP statements and evaluates compiled expressions
// with registered functions, classes, constructors, and runtime state.
type Runtime struct {
	out        io.Writer
	outStack   []io.Writer
	flat       bool
	status     telemetry.State
	entrypoint string
	observers  []Observer
	funcs      map[string]*funcEntry
	userFns    map[string]struct{}
	// funcSites records the site of each user function declaration, so a
	// second one can name the first the way PHP's fatal error does. userFns
	// alone cannot: a set holds no site, and the message is the useful half.
	funcSites map[string]FuncSite
	// hoisted records the programs Run already hoisted, by node identity, so
	// re-running a compiled program on the same runtime is idempotent instead
	// of a redeclaration error. Only the top-level Run entry consults it: an
	// include re-declaring a function is a real PHP error and keeps being one.
	//
	// The value is the filename it hoists under, and Fork
	// replays onto a sibling so it declares the same symbols.
	hoisted map[*model.Program]string
	classes map[string]*model.Class

	// interfaces holds the interface declarations hoist saw, keyed by name.
	// An interface is still no value and no constructor: the table exists so
	// Interface::CONST resolves through the name declaring the constant.
	// Implementing classes take nothing from it (docs/design.md).
	interfaces map[string]*model.InterfaceDecl

	// lazyGlobals binds one superglobal on its first read. Context.Register
	// installs it per request in place of building all seven views eagerly,
	// because a page reads one superglobal or none; see Runtime.resolveAuto.
	lazyGlobals func(name string) any

	// auto holds the request superglobals in fixed fields; see
	// phpval.AutoGlobals for why they are not globals map entries.
	auto phpval.AutoGlobals

	// sgScratch holds the superglobal containers this runtime rebuilds per
	// request (_SERVER, _ENV, _REQUEST, _FILES, argv), reused across
	// requests through model.Array.Reset so the buckets and slices a request
	// grew serve the next one. Scripts alias arrays by reference, but every
	// alias dies with its session; a Go binding that kept one across
	// requests broke the borrowed-arguments contract already.
	sgScratch map[string]*model.Array
	// hostFlat is the runtime's bytecode host, created on the first flat run;
	// see runFlat.
	hostFlat *flatHost

	// Env is the environment visible to PHP for this Runtime. New snapshots the
	// host environment so mutations remain local to a single request/runtime.
	Env map[string]string

	// constructors maps a class name to a Go constructor function, so PHP's
	// `new Name` instantiates a native Go value in place of a model.Object. This
	// is the bridge for "bring your own type": a constructor like
	// func(ctx context.Context) (Storage, error) makes `$s = new Storage;`
	// behave like `s, err := NewStorage(ctx)` (the error surfaces as a throw).
	constructors map[string]*funcEntry
	autoloaders  []any
	includePath  string

	// ctx is the request/lifecycle context auto-injected into any registered
	// callable (constructor, method, function) whose first parameter is a
	// context.Context, mirroring vuego's wrapContextFunc. It lets PHP call
	// these symbols without supplying the context argument explicitly.
	//
	// bindContext derives it, nothing assigns it: SetContext records client
	// below, and set_time_limit and ignore_user_abort shape it. See
	// runtime_deadline.go.
	ctx context.Context

	// host is the context the host set, and ctx derives from it. client is
	// the connection a handler answers right now: host, until a handler
	// enters a request. connection_aborted() reads client, so it survives
	// ignore_user_abort detaching ctx from the disconnect. inRequest reports
	// a request being live, which is when a binding's context derives from
	// client in place of ctx; see runtime_deadline.go and bindingContext.
	host      context.Context
	client    context.Context
	inRequest bool

	// stopCtx cancels ctx, which bindContext builds once per session and
	// never rebuilds; the timers below cancel through it and derive no new
	// context, because a binding that blocks parks on the one it holds.
	// ctxDone and clientDone are their Done channels, read once and kept so
	// the per-statement check is a select on a field. See
	// runtime_deadline.go.
	stopCtx     context.CancelFunc
	ctxDone     <-chan struct{}
	clientDone  <-chan struct{}
	scriptTimer *time.Timer
	timedOut    atomic.Bool
	timeLimit   time.Duration
	ignoreAbort bool
	watching    bool
	// deadlineArmed is whether any of the above can stop this run, refreshed
	// when one changes so the statement loop reads a field.
	deadlineArmed bool

	// prepare is how the host installed this runtime's bindings, recorded so
	// Fork can install them on a child the same way. Copying the tables is not
	// enough: a binding closes over its registering runtime, so a copied one
	// reads and writes the parent. See SetPreparer.
	prepare func(*Runtime)

	// profile is the area mask this runtime's bindings install under, so
	// that rerooting them later installs no area the mount left out. New sets
	// every area: a runtime nobody narrowed has the whole surface.
	profile Profile

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

	// frozenEnv snapshots Env the way frozenConsts snapshots constants: a
	// reset restores it, so a putenv() belongs to the program that called it.
	// envShared marks Env as being the snapshot itself, which PutEnv copies
	// before its first write. Nil until FreezeStdlib, which leaves a CLI
	// runtime's Env mutable in place.
	frozenEnv map[string]string
	envShared bool

	// resets run inside resetExecution, so a binding keeping state of its own
	// beside the runtime (the ob_* buffer stack) drops it when the runtime is
	// reset between programs. See OnReset.
	resets []func()

	// opts holds runtime source root, working directory, and write policy.
	// opts.WorkDir is the one field a script can move, through chdir; workDirBase
	// is what the host configured, so a reset session starts where the first one
	// did, and not wherever the last one wandered to.
	opts         Options
	workDirBase  string
	includeCache *IncludeCache
	included     []string

	// lookupScanned records that a Lookup has already considered parsing the
	// source root into the include cache, so Lookup walks a tree that yielded
	// nothing once, not once per symbol. See Runtime.scanTree.
	lookupScanned bool

	// symbols is the last index of the tree's free functions, held against the
	// include cache version symbolIndex built it at, so binding a second
	// handler does not walk the application again. See Runtime.symbolIndex.
	symbols        *symbolTable
	symbolsVersion uint64

	// classConsts caches evaluated class constants (Class::NAME) per class.
	classConsts map[string]map[string]any

	// classStatics holds the live value of every `static $name` property, one
	// bag per class. Unlike a constant a static is mutable and shared, so the
	// bag is the storage itself and no cache: the declaration's default seeds
	// it the first time a script touches the class, and every later read and
	// write of Class::$name resolves through it.
	classStatics map[string]map[string]any

	// funcStatics holds the live bindings of function-level `static $x`
	// declarations, one bag per StaticVar statement. The statement node's
	// address is the function identity: the parser builds the AST once, and
	// each `static` statement sits in exactly one function body. Closures do
	// not use this table: each closure value carries its own bag on
	// closureEnv, which is PHP's per-instance static semantics.
	funcStatics map[*model.StaticVar]map[string]any

	exprCache *ExprCache
	compiled  map[model.Expr]*compiledExpr
	// concatParts caches the flattened operand list of a top-level `.`
	// expression by node identity, the way compiled caches programs: the
	// tree shape never changes, and reflattening it allocated a slice per
	// evaluation of every echo with a concatenation in it.
	concatParts map[*model.Binary][]model.Expr
	// fileDirs caches path.Dir per loaded filename; see Runtime.fileDir.
	fileDirs map[string]string

	// boxedStrings holds each magic constant and class name as an interface
	// value, boxed once; see Runtime.boxed.
	boxedStrings map[string]any
	helpers      map[string]func(...any) (any, error)

	// envMu guards envFree, the free list of reusable evaluation environments,
	// and scopeFree, the free list of call scopes user-function invocations
	// reuse; maxFreeEnvs bounds both.
	envMu     sync.Mutex
	envFree   []*evalEnv
	scopeFree []*Scope

	// goMethods caches Go method resolution per (receiver type, spelled name):
	// the method index, its bound signature and whether it takes a context.
	// MethodByName plus the case-insensitive fallback plus Type() cost an
	// allocation per call otherwise, which a method loop pays per iteration.
	// The cache holds misses too, so a probe does not rescan the method set.
	goMethods map[goMethodKey]goMethodInfo

	// goFields caches struct field resolution per (struct type, spelled
	// name), for the same reason goMethods exists: fieldByNameFold walks the
	// field set through two FieldByNameFunc closures on every property read
	// otherwise, and a handler reads $r->method per request. The cache holds
	// misses too.
	goFields map[goMethodKey]goFieldInfo

	sourceSpans map[model.Stmt]model.SourceSpan
	// currentStmt is the statement exec is running, for the two telemetry
	// sites that attribute a span to a source line. The field holds the
	// statement and not the line because an interface keys sourceSpans:
	// storing the node is one word and resolving it through the map is a hash
	// of an interface, which belongs on the error and tracing paths and not on
	// every statement executed.
	currentStmt model.Stmt

	// coverage counts statement executions when a host installed a collector
	// with SetCoverage. Nil means off, which is the only cost the common path
	// pays for it.
	coverage *coverage.Collector

	// preludeDone records that the runtime included Options.Include, so it
	// installs the declarations that file carries once and redeclares none.
	// A runtime serves one request, so this is once per request; ResetSession
	// clears it with everything else a reused runtime drops between them.
	preludeDone bool

	// frames is the stack of live interpreter frames, global frame first;
	// vmWalkers walk the live values of every flat VM now running.
	// Together with globals and classStatics they are the roots MemoryWalk
	// measures.
	frames    []*Scope
	vmWalkers []func(yield func(any))

	memBase int64 // host request overhead, accounted by AccountRequest
	// infoSections are the blocks phpinfo() prints after the runtime's own,
	// contributed by whatever holds memory across requests.
	infoSections []namedInfoSection
	memUsage     int64 // cached result of the last MemoryWalk
	memPeak      int64 // high-water mark, refreshed at every walk
	memTick      int   // statements since the last checkpoint walk
	memPending   int64 // shallow bytes host calls produced since the last walk
}

// maxFreeEnvs bounds the per-Runtime free list of evaluation environments. Each
// entry retains one closure per registered function, so the list stays small;
// nesting deeper than this simply builds (and discards) an environment the way
// every Eval used to.
const maxFreeEnvs = 16

// envSizeHint presizes an evaluation environment's map. The size it presizes to
// is what an environment holds:
// since installFunc adds registered functions on demand, an environment carries
// the ~16 PHP-semantic helpers, the functions its expressions actually call, and
// a handful of layered per-expression keys. The hint only has to avoid the first
// few rehashes; an environment that outgrows it grows once and is then reused.
const envSizeHint = 48

// scopeRef lets buildEnv build the environment's closures once while they
// still see the scope of the evaluation using them. Helpers read ref.scope
// at call time; anything that outlives the evaluation (a bound method, a
// by-reference setter) copies the scope out of the ref first.
type scopeRef struct {
	scope *Scope
}

// evalEnv is one reusable expr environment: the registered functions and the
// PHP-semantic helpers, built once per function-table generation, plus the
// per-expression keys layered on top by Eval and removed again on release.
type evalEnv struct {
	ref   *scopeRef
	env   map[string]any
	exprs map[string]model.Expr
	built bool
	// vars is the pooled slot buffer the closure engine reads variables
	// from, and cenv the reused carrier handed to expr.Run; both exist so
	// the slot path allocates nothing per evaluation.
	vars []any
	cenv expr.Env
}

// slots returns the pooled slot buffer sized to n. releaseEnv cleared it, so
// an evaluation sees only what it wrote.
func (st *evalEnv) slots(n int) []any {
	if cap(st.vars) < n {
		st.vars = make([]any, n)
	}
	st.vars = st.vars[:n]
	return st.vars
}

// SAPI returns the configured SAPI string.
func (rt *Runtime) SAPI() string {
	return rt.opts.SAPI
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
// While a redirection is active (PushOutput, which output buffering builds
// on) this is the innermost writer, so it captures everything the script
// emits and passes none of it on.
func (rt *Runtime) Output() io.Writer {
	if n := len(rt.outStack); n > 0 {
		return rt.outStack[n-1]
	}
	return rt.out
}

// emptyStdin is the input stream a runtime with no stdin of its own reads:
// nothing, from any offset.
//
// One value, shared by every runtime and every reset, because it holds no state
// to share. A strings.Reader over "" carries a read offset, which makes it a
// value per use; this carries nothing, and a zero-size value costs no allocation
// to put in an interface. The two readers of Runtime.Stdin - STDIN in
// stdlib/core and php://input in stdlib/files - take it as an io.Reader and ask
// nothing else of it.
type emptyStdin struct{}

func (emptyStdin) Read([]byte) (int, error) { return 0, io.EOF }

// New presizes each table to the size the stdlib fills it to.
//
// A table starting empty rehashes its way up: `phpscript server` builds a
// runtime and registers the whole stdlib into it per request, and over half the
// time inside mapassign on that path went to rehash. The numbers are what the
// `info` command reports as Internal Functions, Declared Classes and Defined
// Constants, rounded up to leave room for what a host and a script add. They
// are a hint and nothing reads them back, so one that drifts low costs a rehash
// and nothing else.
const (
	stdlibFuncs   = 384
	stdlibClasses = 48
	stdlibConsts  = 192
)

// New returns a Runtime that writes echo output to w (defaults to os.Stdout).
func New(w io.Writer, opts Options) *Runtime {
	if w == nil {
		w = os.Stdout
	}
	if opts.Stdin == nil {
		opts.Stdin = emptyStdin{}
	}
	opts.WorkDir = cleanFSPath(opts.WorkDir)
	rt := &Runtime{
		out:          w,
		Env:          ScriptEnvironment(opts.Env),
		opts:         opts,
		profile:      ^Profile(0),
		includeCache: NewIncludeCache(),
		funcs:        make(map[string]*funcEntry, stdlibFuncs),
		userFns:      map[string]struct{}{},
		funcSites:    map[string]FuncSite{},
		hoisted:      map[*model.Program]string{},
		workDirBase:  opts.WorkDir,
		classes:      make(map[string]*model.Class, stdlibClasses),
		interfaces:   map[string]*model.InterfaceDecl{},
		constructors: make(map[string]*funcEntry, stdlibClasses),
		includePath:  ".",
		ctx:          context.Background(),
		host:         context.Background(),
		client:       context.Background(),
		globals:      map[string]any{},
		constants:    make(map[string]any, stdlibConsts),
		classConsts:  map[string]map[string]any{},
		classStatics: map[string]map[string]any{},
		funcStatics:  map[*model.StaticVar]map[string]any{},
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
			"__cmp":        adapt(phpCompare),
			"__bit":        adapt(phpBitwise),
			"__bitnot":     adapt(phpBitNot),
			"__instanceof": adapt(phpInstanceOf),
			"__neg":        adapt(phpNegate),
		},
		memUsage: runtimeBaseline,
	}
	rt.registerCompiledTreeInfo()
	return rt
}

// InfrastructurePrefix names the variables that configure phpscript and the
// platform it runs on: connection strings, the listen address, the telemetry
// block. They are the host's configuration and getenv() does not answer them,
// so a tenant cannot read the operator's connection strings.
const InfrastructurePrefix = "PLATFORM_"

// ScriptEnvironment returns the environment scripts read with getenv().
//
// A nil environment means the process environment, as a CLI run has it. A host
// that configured one passes it instead, and a virtual host always does, so
// that a site sees the variables it declared and none of the rest the
// operator's process happens to carry. Either way ScriptEnvironment drops the
// infrastructure variables.
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

// FreezeStdlib snapshots constants and the environment after the host
// registers its bindings, so ResetSession can drop what a script defined
// without losing what the host configured.
func (rt *Runtime) FreezeStdlib() {
	rt.frozenConsts = rt.constants
	rt.constsShared = true
	rt.frozenEnv = rt.Env
	rt.envShared = true
}

// ResetSession prepares the runtime to execute another program: new output and
// stdin, empty globals and user declarations. Host functions, constructors and
// the expression/bytecode caches stay.
func (rt *Runtime) ResetSession(out io.Writer, stdin io.Reader) {
	rt.resetExecution(out, stdin)
	// The closures a hoist installed have to come out of the function table
	// too, not just out of the name set. Leaving them behind made
	// function_exists answer for a function the reset session never declared,
	// and made the redeclaration check see the previous session's work as a
	// binding it must not shadow.
	for name := range rt.userFns {
		delete(rt.funcs, name)
	}
	clear(rt.userFns)
	clear(rt.funcSites)
	clear(rt.hoisted)
	rt.included = nil
	clear(rt.classes)
	clear(rt.interfaces)
}

// resetExecution drops what one program wrote and keeps what declared it: the
// function and class tables, and the record of loaded files.
//
// This is ResetSession without the forgetting, for a runtime a host hands to
// one request after another - Pool.Put is the caller. A fork's declarations
// came from its parent's tree and are the reason it exists; dropping them
// between requests would leave it with nothing to answer.
func (rt *Runtime) resetExecution(out io.Writer, stdin io.Reader) {
	if out == nil {
		out = os.Stdout
	}
	rt.out = out
	rt.releaseOutput()
	// Bindings holding state beside the runtime drop it with the output they
	// mirror: an ob_* level a program left open is its text and its level, and
	// both would read back in the next program.
	for _, reset := range rt.resets {
		reset()
	}
	// The next program starts unobserved the way a fresh runtime does. A
	// request that ended in StateError is not the next request's opening
	// status, which SetContext would otherwise publish.
	rt.status = ""
	if stdin == nil {
		stdin = emptyStdin{}
	}
	rt.opts.Stdin = stdin
	rt.opts.WorkDir = rt.workDirBase
	rt.preludeDone = false
	if rt.frozenEnv != nil {
		rt.Env = rt.frozenEnv
		rt.envShared = true
	}
	clear(rt.globals)
	rt.auto.Reset()
	rt.lazyGlobals = nil
	// The scratch containers stay for the next Register, emptied now so a
	// reset session retains nothing of the last request's values.
	for _, arr := range rt.sgScratch {
		arr.Reset()
	}
	rt.shutdown = nil
	rt.autoloaders = nil
	// Dropped with the rest: an AST node held past the run that executed it
	// keeps the previous program's tree alive.
	rt.currentStmt = nil
	clear(rt.classConsts)
	clear(rt.classStatics)
	clear(rt.funcStatics)
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
	rt.resetLimits()
}

// Reset returns the runtime to the state a new one is in, so a pool can hand
// the same value to the next program, with no second one built.
//
// Reset is ResetSession plus what outlives a session: the parse caches and the
// two maps keyed by AST node, which ResetSession keeps because a --count loop
// runs one program and the keys are the same nodes each time. Across two
// programs they hold the previous tree alive. Reset clears the maps and never
// replaces them, so the next program starts on the buckets they grew.
func (rt *Runtime) Reset(out io.Writer, stdin io.Reader) {
	rt.ResetSession(out, stdin)
	clear(rt.compiled)
	clear(rt.sourceSpans)
	rt.includeCache.Clear()
	rt.exprCache.Clear()
	// The tree a lookup indexed went with the include cache, so the next one
	// parses it again, and resolves against nothing otherwise.
	rt.lookupScanned = false
	// The index is version-guarded and would rebuild on its own, but it holds a
	// program pointer per symbol, which is the tree Reset exists to let go of.
	rt.symbols = nil
}

// SetContext installs the lifecycle context auto-injected into registered Go
// callables whose first parameter is a context.Context (constructors, methods,
// functions). Defaults to context.Background().
func (rt *Runtime) SetContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Recorded as the client, then derived: a limit already set survives a host
	// swapping the context, and the derived one is what everything else reads.
	rt.host, rt.client = ctx, ctx
	rt.clientDone = watchEnd(ctx)
	rt.bindContext()
	for _, observer := range rt.observers {
		observer.UpdateStatus(rt.ctx, rt.status)
	}
}

// Context returns the configured lifecycle context.
func (rt *Runtime) Context() context.Context {
	return rt.ctx
}

// Observer receives lifecycle updates for a Runtime. Implementations must be
// safe for use by concurrent runtimes. Trace returns a span so the interpreter
// can measure the region it just reported; a nil span is valid and every
// method on it does nothing, so an observer with no trace in the context can
// return one.
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

// UpdateStatus publishes a lifecycle phase to all registered observers. Hosts
// call it too, for phases that occur outside PHP execution.
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

// MemoryPeak returns the high-water usage mark. MemoryWalk samples it at walk
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

// CompiledTree reports what the parsed and compiled source tree costs this
// runtime: cached programs, compiled expressions, and the heap a precompile
// pass measured itself adding, which is zero until one runs.
//
// memory_limit counts none of it. Every request shares the tree and it
// outlives them all, and php draws the same line by keeping a compiled script
// under opcache.memory_consumption.
func (rt *Runtime) CompiledTree() (programs, expressions int, heap int64) {
	return rt.includeCache.Len(), rt.exprCache.Len(), rt.includeCache.Precompiled()
}

// Precompiled reports whether this runtime precompiles.
func (rt *Runtime) Precompiled() bool { return rt.opts.Precompile }

// AccountRequest folds the size of host-owned request-lifetime values into
// the baseline the memory walk starts from: the request Context, the parsed
// *http.Request, the response writer. A host calls it once per value at the
// point a request crosses into the runtime; the walk then adds live script
// values on top. The baseline is an estimate of what the request costs before
// any PHP evaluates, and no audit of every host allocation.
func (rt *Runtime) AccountRequest(values ...any) {
	visited := make(visitedSet)
	for _, v := range values {
		rt.memBase += DeepSize(v, visited)
	}
}

// MemoryWalk recomputes live usage from the roots: the interpreter frame
// stack, globals, class statics, and any running flat VM's live values.
// A visited set keyed on container identity counts a value reachable through
// more than one variable once and terminates cycles. The result refreshes the
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
	rt.auto.Range(func(_ string, val any) {
		total += DeepSize(val, visited)
	})
	for _, bag := range rt.classStatics {
		for name, val := range bag {
			total += 16 + int64(len(name)) + DeepSize(val, visited)
		}
	}
	for _, bag := range rt.funcStatics {
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
// record this once, with no walk apiece.
func (rt *Runtime) lastMemoryUsage() int64 {
	if rt.memUsage < runtimeBaseline {
		return runtimeBaseline
	}
	return rt.memUsage
}

// checkMemory walks and reports a RuntimeException when usage exceeds the
// configured MemoryLimit. Without a limit it does nothing.
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

// scratchArray returns the reusable container registered under name,
// emptied and ready to refill.
func (rt *Runtime) scratchArray(name string) *model.Array {
	if arr, ok := rt.sgScratch[name]; ok {
		arr.Reset()
		return arr
	}
	if rt.sgScratch == nil {
		rt.sgScratch = map[string]*model.Array{}
	}
	arr := model.NewArray()
	rt.sgScratch[name] = arr
	return arr
}

// acquireScope returns a cleared scope from the free list, or a fresh one.
// Pair with releaseScope at a call boundary whose scope cannot outlive the
// call: expression evaluation inside the body holds it only through
// scopeRef, which releaseEnv nils, and a closure captures values, never the
// scope itself.
func (rt *Runtime) acquireScope() *Scope {
	rt.envMu.Lock()
	defer rt.envMu.Unlock()
	if n := len(rt.scopeFree) - 1; n >= 0 {
		scope := rt.scopeFree[n]
		rt.scopeFree[n] = nil
		rt.scopeFree = rt.scopeFree[:n]
		return scope
	}
	return NewScope()
}

func (rt *Runtime) releaseScope(scope *Scope) {
	rt.envMu.Lock()
	defer rt.envMu.Unlock()
	if len(rt.scopeFree) >= maxFreeEnvs {
		return
	}
	clear(scope.vars)
	clear(scope.deferred[:cap(scope.deferred)])
	scope.deferred = scope.deferred[:0]
	scope.args = nil
	scope.statics = nil
	rt.scopeFree = append(rt.scopeFree, scope)
}

// Trace publishes a trace span to registered observers and returns the first
// mutable span one of them created.
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
	// rootPath writes the entrypoint from the source filesystem's root, which
	// is how a script reads every path: __DIR__ . "/x" names one file wherever
	// chdir has moved the working directory, and a span, a coverage key and a
	// redeclaration message all spell the same file the same way.
	rt.entrypoint = rootPath(filename)
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
// context.Context (auto-injected) and may return a trailing error, which
// reaches the script as a thrown error. When a direct variable assignment
// receives a constructed value that implements SetID(string), the runtime
// passes it the PHP variable name without the leading dollar sign.
// Example:
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

// SetCoverage installs a statement-coverage collector; nil turns collection
// off. Coverage is an interpreter feature: with a collector in place, a
// runtime created with NewFlatStack stops delegating to the bytecode backend,
// because flatstack carries no coverage support (docs/flatstack.md) and the
// fallback is atomic: a program partly counted would be a program partly run.
func (rt *Runtime) SetCoverage(c *coverage.Collector) { rt.coverage = c }

// SetIncludeCache installs a shared include cache. Only runtimes whose include
// paths resolve within the same source-root namespace may share one. Passing
// nil disables include caching.
func (rt *Runtime) SetIncludeCache(cache *IncludeCache) { rt.includeCache = cache }

// SetExprCache installs a source-keyed compiled-program cache that is safe to
// share across runtimes. AST-specific expression metadata remains runtime-local.
// Passing nil disables cross-runtime expression caching.
func (rt *Runtime) SetExprCache(cache *ExprCache) { rt.exprCache = cache }

// FS returns the configured source root (or nil).
func (rt *Runtime) FS() fs.FS { return rt.opts.RootFS }

// Stdin returns the input stream configured by the runtime host.
func (rt *Runtime) Stdin() io.Reader { return rt.opts.Stdin }

// WorkDir returns the working directory as an fs.FS path inside the source
// root: "." for the root itself, "app" for a directory below it. Bindings that
// resolve a path against it want this spelling.
func (rt *Runtime) WorkDir() string { return rt.opts.WorkDir }

// WorkDirPath returns the working directory the way a script reads it, written
// from the source filesystem's root. This is what getcwd() answers with.
func (rt *Runtime) WorkDirPath() string { return rootPath(rt.opts.WorkDir) }

// ResolvePath maps a path a script supplied onto the source filesystem, through
// the working directory. An include resolves through the same function,
// exported so a binding taking a path from PHP reaches the same file: a
// runtime where include "x.php" and file_get_contents("x.php") name different
// files would be worse than either rule on its own.
func (rt *Runtime) ResolvePath(p string) string { return rt.resolveFSPath(p) }

// SetWorkDir moves the working directory and reports whether it could. A
// relative path resolves against the current one and an absolute path against
// the source root, and neither climbs out of it.
//
// The directory is per-runtime state, so a script moving it moves nothing
// another request sees and os.Chdir is never called. SetWorkDir returns false
// for a path naming no directory, as php documents.
func (rt *Runtime) SetWorkDir(dir string) bool {
	target := rt.resolveFSPath(dir)
	if target == "" {
		target = "."
	}
	if rt.opts.RootFS != nil {
		info, err := fs.Stat(rt.opts.RootFS, target)
		if err != nil || !info.IsDir() {
			return false
		}
	}
	rt.opts.WorkDir = target
	return true
}

// Database returns the provider named connections resolve through, or nil when
// the host configured none. The binding decides what nil falls back to.
func (rt *Runtime) Database() model.DatabaseProvider { return rt.opts.Database }

// Mail returns the provider named mail servers resolve through, or nil when the
// host configured none. The binding decides what nil falls back to.
func (rt *Runtime) Mail() model.MailProvider { return rt.opts.Mail }

// WritablePaths returns the configured writable path whitelist.
func (rt *Runtime) WritablePaths() []string { return append([]string(nil), rt.opts.WritablePaths...) }

// UploadFileMode returns the mode move_uploaded_file() sets on a stored upload,
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
	if rt.auto.Set(name, val) {
		return
	}
	rt.globals[name] = val
}

// PutEnv writes one variable into the script environment, backing putenv().
// While Env is the frozen snapshot FreezeStdlib took, the write copies it
// first, the way SetConst copies the frozen constants: the snapshot is what a
// reset restores, so a write landing in it would outlive its program.
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

// RegisterShutdown appends a callback to run when the current program exits.
// Shutdown callbacks run in registration order, including after exit or error.
func (rt *Runtime) RegisterShutdown(callback any) {
	rt.shutdown = append(rt.shutdown, callback)
}

// Const returns a registered constant value and whether the table holds it.
//
// The magic constants are not among them, so defined("__FILE__") and
// constant("__FILE__") match php: it compiles those names and neither
// function can see one.
func (rt *Runtime) Const(name string) (any, bool) {
	v, ok := rt.constants[name]
	return v, ok
}

// entrypointConst answers __FILE__ and __DIR__ for source that named no file.
//
// parser.ParseFile compiles both to literals, so a program read from the
// filesystem never asks. That leaves Runtime.Load, which takes source with
// nothing behind it, and there the entrypoint is the best answer there is.
//
// They are not in the constant table. php resolves them when it compiles, so
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
	// This invalidates nothing: evaluation environments resolve the table per
	// call (see installFunc), the direct compiler reads no type env, and the
	// entry builds its invoker on first dispatch, so a re-registration is a
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
	return rt.includeFile(path, false, rt.newScope())
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

// OnError installs an error handler (register_error_handler). When set, the
// runtime routes evaluation errors here and aborts no caller.
func (rt *Runtime) OnError(fn func(error)) {
	rt.errorHandler = fn
}

// OnReset registers fn to run whenever the runtime is reset between programs.
// A binding that keeps state of its own beside the runtime registers one, so a
// host reusing the runtime starts the next program without it; the ob_* buffer
// stack in stdlib/compat is the case that required it.
func (rt *Runtime) OnReset(fn func()) {
	rt.resets = append(rt.resets, fn)
}

// Eval compiles e to a closure chain, binds the referenced variables from
// scope by slot, and runs it.
func (rt *Runtime) Eval(e model.Expr, scope *Scope) (any, error) {
	// A literal is its own value; nothing needs compiling to answer what the
	// parser already knew.
	if l, ok := e.(*model.Lit); ok {
		if n, ok := l.Value.(int); ok {
			// PHP integers are int64 everywhere else in the runtime.
			return int64(n), nil
		}
		return l.Value, nil
	}
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

	st := rt.acquireEnv(scope)
	st.exprs = ce.exprs
	defer rt.releaseEnv(st)

	// installFunc adds a registered function on demand: an environment carries
	// the PHP-semantic helpers plus whatever the expressions evaluated with it
	// have called so far, and no closure per entry of the function table. The
	// installed closures persist across evaluations (see installFunc). An
	// immediately invoked anonymous function appears in ce.calls under its
	// synthetic identifier, which is in no function table; the loop below
	// binds it per evaluation, so installFunc must not stub it as undefined.
	for _, name := range ce.calls {
		if _, ok := ce.closures[name]; ok {
			continue
		}
		rt.installFunc(st, name)
	}

	// Per-expression values bind by slot into a pooled slice; the base env
	// carries only the persistent helpers and installed functions.
	vars := st.slots(ce.prog.NumSlots())
	for id, cl := range ce.closures {
		slot, ok := ce.closureSlots[id]
		if !ok {
			continue
		}
		vars[slot] = rt.newClosure(cl, captureClosureEnv(cl, scope))
	}
	for i, name := range ce.vars {
		v, err := rt.resolveVar(name, ce.idents[i], scope)
		if err != nil {
			return nil, err
		}
		if slot := ce.varSlots[i]; slot >= 0 {
			vars[slot] = v
		}
	}
	st.cenv.Vars = vars
	st.cenv.Base = st.env
	return expr.Run(ce.prog, &st.cenv)
}

// resolveVar returns the value Eval binds for one variable entry. Bare
// identifiers that are not set in the current scope fall back to the constant
// table (PHP constants are visible in every scope, whereas plain variables
// belong to their frame), then the superglobals. A bare name nothing
// defines is an error - PHP 8 raises Error there, while an unset variable of
// the same spelling stays null, so the two carry different
// identifiers.
// helperVar reads a variable or bare name from the live scope at evaluation
// time. The closure engine reads it in place of the slot snapshot inside
// expressions that contain a marked sub-expression, whose scope writes a
// snapshot taken before the run would miss.
func (rt *Runtime) helperVar(ref *scopeRef) func(string, bool) (any, error) {
	return func(name string, isConst bool) (any, error) {
		ident := varIdent(name)
		if isConst {
			ident = constIdent(name)
		}
		return rt.resolveVar(name, ident, ref.scope)
	}
}

func (rt *Runtime) resolveVar(name, ident string, scope *Scope) (any, error) {
	if v, ok := scope.Get(name); ok {
		return v, nil
	}
	if c, ok := rt.entrypointConst(name); ok {
		return c, nil
	}
	if c, ok := rt.constants[name]; ok {
		return c, nil
	}
	if v, ok := rt.resolveAuto(name); ok {
		return v, nil
	}
	if strings.HasPrefix(ident, constIdentPrefix) {
		return nil, &UndefinedConstantError{Name: name}
	}
	return nil, nil
}

// resolveAuto answers a superglobal read: the bound value, or the view the
// request's lazy binder builds on the first read of that name. It binds what
// the binder answers, so the next read is one field read, and a write through
// the view reaches every later reader the way an eagerly bound view did.
func (rt *Runtime) resolveAuto(name string) (any, bool) {
	if v, ok := rt.auto.Lookup(name); ok {
		return v, true
	}
	if rt.lazyGlobals == nil || !rt.auto.Claims(name) {
		return nil, false
	}
	v := rt.lazyGlobals(name)
	if v == nil {
		return nil, false
	}
	rt.auto.Set(name, v)
	return v, true
}

func (rt *Runtime) compileExpr(e model.Expr) (*compiledExpr, error) {
	if rt.compiled != nil {
		if ce, ok := rt.compiled[e]; ok {
			return ce, nil
		}
	}

	if dc, ok := rt.exprCache.GetExpr(e); ok {
		ce := newDirectCompiledExpr(dc)
		rt.setCompiledExpr(e, ce)
		return ce, nil
	}
	dc, err := expr.CompileExpr(e, exprHelpers)
	if err != nil {
		return nil, err
	}
	rt.exprCache.SetExpr(e, dc)
	ce := newDirectCompiledExpr(dc)
	rt.setCompiledExpr(e, ce)
	return ce, nil
}

func (rt *Runtime) setCompiledExpr(e model.Expr, ce *compiledExpr) {
	if rt.compiled == nil {
		rt.compiled = make(map[model.Expr]*compiledExpr)
	}
	rt.compiled[e] = ce
}

// acquireEnv returns an evaluation environment bound to scope. Evaluations
// reuse environments: buildEnv builds the expensive part, one closure per
// registered function plus the PHP-semantic helpers, once per function-table
// generation, and every later call only rebinds the scope.
func (rt *Runtime) acquireEnv(scope *Scope) *evalEnv {
	rt.envMu.Lock()
	var st *evalEnv
	if n := len(rt.envFree); n > 0 {
		st = rt.envFree[n-1]
		rt.envFree[n-1] = nil
		rt.envFree = rt.envFree[:n-1]
	}
	rt.envMu.Unlock()

	if st == nil {
		st = &evalEnv{ref: &scopeRef{}, env: make(map[string]any, envSizeHint)}
	}
	// Built once per environment, for the runtime's lifetime: the helpers
	// close over the stable scope reference and installed functions resolve
	// per call, so a function-table change invalidates nothing here.
	if !st.built {
		rt.buildEnv(st)
	}
	st.ref.scope = scope
	return st
}

// buildEnv fills st with the env shared by every evaluation: the PHP-semantic
// helpers. The engine carries no builtins of its own, so a registered function
// resolves as a bare identifier the engine calls by name, but buildEnv
// installs *no* such function: installFunc adds the ones an expression
// actually calls, on demand (see Eval). Eval layers variables and other
// per-expression keys on top.
//
// adapt() wraps every function into a uniform func(...any) (any, error)
// signature. This lets the compile-time type env accept dynamically typed PHP
// arguments and keeps the runtime func type in sync with the type env (see
// helpers.go::adapt and Eval).
//
// The closures read the scope through st.ref and capture nothing, which is
// what makes the environment reusable.
func (rt *Runtime) buildEnv(st *evalEnv) {
	env := st.env
	clear(env)
	ref := st.ref
	// PHP-semantic helpers (see helpers.go). They close over rt and the scope
	// reference so that method dispatch and instantiation can re-enter the
	// interpreter.
	for name, fn := range rt.helpers {
		env[name] = fn
	}
	env["__var"] = adapt(rt.helperVar(ref))
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
	env["__callable"] = adapt(rt.helperCallable(ref))
	env["__callablemember"] = adapt(rt.helperCallableMember(ref))
	// Expression markers (`__eval`) resolve against the expression now
	// evaluated with this environment, which Eval stores on st.
	env["__eval"] = adapt(func(id string) (any, error) {
		e := st.exprs[id]
		if e == nil {
			return nil, fmt.Errorf("eval: unknown expression marker %s", id)
		}
		return rt.Eval(e, ref.scope)
	})
	// func_get_args() needs the current frame's arguments, so buildEnv installs
	// it as a scope-aware helper and not a plain forwarded function.
	// The frame already holds its arguments as a []any, which the VM indexes and
	// iterates directly, so func_get_args() returns that slice and not
	// rebuilding it as an *model.Array.
	env["func_get_args"] = adapt(func() []any { return funcGetArgs(ref.scope) })
	st.built = true
}

// installFunc adds the registered function name to st's environment, unless it
// already carries one. The closure stays in the environment (releaseEnv strips
// only the per-expression keys), so installFunc wraps a function at most
// once per environment per function-table generation: buildEnv clears the map
// whenever the generation moves, the one point at which a name could have been
// re-registered with a different implementation.
//
// A name that is in no function table installs a stub that reports the call as
// undefined, so this engine and flatstack (see helperFunc) report the same
// message to a script where the VM would say "cannot call nil". The stub
// belongs to the current generation like any other entry: declaring the
// function afterwards re-registers it, which moves the generation and
// rebuilds the environment.
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
	// Resolution happens per call, not at install. The closure then survives
	// every function-table change - a script re-declaring its functions on
	// each run, a conditionally declared function appearing after the first
	// call reported it undefined, so a built environment
	// lives for the runtime's lifetime and is never rebuilt on every
	// registration.
	st.env[name] = func(args ...any) (any, error) {
		entry, ok := rt.lookupEntry(name)
		if !ok {
			return nil, fmt.Errorf("call to undefined function %s()", name)
		}
		result, err := rt.invokeEntry(entry, args, ref.scope)
		return result, nameCallError(err, name)
	}
}

// releaseEnv clears the per-expression slots, drops the references the
// environment held to the scope and expression, and returns it to the free
// list.
func (rt *Runtime) releaseEnv(st *evalEnv) {
	st.exprs = nil
	st.ref.scope = nil
	for i := range st.vars {
		st.vars[i] = nil
	}
	st.vars = st.vars[:0]
	// The arena's backing array survives the release; its values do not. The
	// length is zero whenever the calls balanced, so the clear covers what a
	// panic left behind and the references parked beyond the length.
	scratch := st.cenv.Scratch
	clear(scratch[:cap(scratch)])
	st.cenv = expr.Env{Scratch: scratch[:0]}

	rt.envMu.Lock()
	if len(rt.envFree) < maxFreeEnvs {
		rt.envFree = append(rt.envFree, st)
	}
	rt.envMu.Unlock()
}

// helperSet implements assignment used as an expression (AssignExpr with a Var
// target): it mutates the current scope and returns the assigned value so the
// surrounding expression (e.g. a comparison) can use it.
func (rt *Runtime) helperSet(ref *scopeRef) func(name string, val any) (any, error) {
	return func(name string, val any) (any, error) {
		ref.scope.Set(name, val)
		return val, nil
	}
}

func (rt *Runtime) evalIncDec(n *model.Unary, scope *Scope) (any, error) {
	cur, err := rt.readLValue(n.X, scope)
	if err != nil {
		return nil, err
	}
	next := phpval.Increment(cur)
	if n.Op == "--" {
		next = phpval.Decrement(cur)
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
	parts, ok := rt.concatParts[n]
	if !ok {
		parts = flattenConcat(n, nil)
		if rt.concatParts == nil {
			rt.concatParts = make(map[*model.Binary][]model.Expr)
		}
		rt.concatParts[n] = parts
	}
	return rt.joinParts(parts, scope)
}

// joinParts evaluates each part and joins their PHP string forms. It serves
// the concat fast path above; an interpolated literal compiles instead, in
// expr.compileInterp, because a per-part Eval dispatch at every execution
// measured 21% slower than the compiled closure (see the Interp benchmarks).
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
		if entry, ok := rt.lookupEntry(name); ok {
			result, err := rt.invokeEntry(entry, args, scope)
			return result, nameCallError(err, name)
		}
		if fallback != "" {
			if entry, ok := rt.lookupEntry(fallback); ok {
				result, err := rt.invokeEntry(entry, args, scope)
				return result, nameCallError(err, fallback)
			}
			// Frame-aware builtins live in the evaluation environment rather
			// than the function table, so the bare-name fast path finds them
			// but this dispatch would not. Inside a namespaced file every
			// global call arrives here, so this dispatch resolves them.
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

// ScopeBuiltins names the builtins installed per-environment, outside
// the function table. function_exists() and the linter both read the function
// table, which does not carry these, so a script calling one reads as a call to
// something undefined without this list.
func ScopeBuiltins() []string {
	return []string{"func_get_args"}
}

// scopeBuiltin resolves the builtins installed per-environment, outside
// through the function table, under the name a PHP script calls them by.
func scopeBuiltin(name string, scope *Scope) (any, bool) {
	if strings.EqualFold(name, "func_get_args") {
		return funcGetArgs(scope), true
	}
	return nil, false
}

// funcGetArgs returns the arguments of the frame scope belongs to.
func funcGetArgs(scope *Scope) []any {
	return scope.args
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

// helperRef returns a setter for a by-reference output parameter (e.g.
// preg_match_all's $matches). The shim calls it to write back into scope.
func (rt *Runtime) helperRef(ref *scopeRef) func(name string) func(any) {
	return func(name string) func(any) {
		// A shim takes the setter and may call it after this expression
		// finishes, so it binds the scope value and not the reference.
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
		// time, so the class need not exist, and composer's generated
		// autoloader relies on that.
		if name == "class" {
			return class, nil
		}
		if !rt.hasClass(class) {
			if _, ok := rt.lookupInterface(class); !ok {
				if err := rt.autoload(class, scope); err != nil {
					return nil, err
				}
			}
		}
		var className string
		var consts []model.Field
		if c, ok := rt.lookupClass(class); ok {
			className, consts = c.Name, c.Consts
		} else if id, ok := rt.lookupInterface(class); ok {
			// An interface holds its constants under its own name. Only this
			// direct spelling resolves; an implementing class acquires none
			// of them (docs/design.md).
			className, consts = id.Name, id.Consts
		} else {
			return nil, fmt.Errorf("class constant %s::%s: unknown class", class, name)
		}
		class = className
		if cached, ok := rt.classConsts[class]; ok {
			if v, ok := cached[name]; ok {
				return v, nil
			}
		}
		for _, k := range consts {
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
