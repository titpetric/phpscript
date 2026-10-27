package runner

import (
	"context"
	"io"
	"io/fs"
	"os"
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
// request, visible in every scope and writing the variable itself rebinds the
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
	// funcSites records the site of each user function declaration and a
	// second one can name the first the way PHP's fatal error does. userFns
	// alone cannot: a set holds no site, and the message is the useful half.
	funcSites map[string]FuncSite
	// hoisted records the programs Run already hoisted, by node identity and
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

	// constructors maps a class name to a Go constructor function and PHP's
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
	// enters a request. connection_aborted() reads client and it survives
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
	// enough: a binding closes over its registering runtime and a copied one
	// reads and writes the parent. See SetPreparer.
	prepare func(*Runtime)

	// profile is the area mask this runtime's bindings install under and
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
	// reset restores it and a putenv() belongs to the program that called it.
	// envShared marks Env as being the snapshot itself, which PutEnv copies
	// before its first write. Nil until FreezeStdlib, which leaves a CLI
	// runtime's Env mutable in place.
	frozenEnv map[string]string
	envShared bool

	// resets run inside resetExecution and a binding keeping state of its own
	// beside the runtime (the ob_* buffer stack) drops it when the runtime is
	// reset between programs. See OnReset.
	resets []func()

	// opts holds runtime source root, working directory, and write policy.
	// opts.WorkDir is the one field a script can move, through chdir; workDirBase
	// is what the host configured and a reset session starts where the first one
	// did, and not wherever the last one wandered to.
	opts         Options
	workDirBase  string
	includeCache *IncludeCache
	included     []string

	// lookupScanned records that a Lookup has already considered parsing the
	// source root into the include cache and Lookup walks a tree that yielded
	// no result once, not once per symbol. See Runtime.scanTree.
	lookupScanned bool

	// symbols is the last index of the tree's free functions, held against the
	// include cache version symbolIndex built it at and binding a second
	// handler does not walk the application again. See Runtime.symbolIndex.
	symbols        *symbolTable
	symbolsVersion uint64

	// classConsts caches evaluated class constants (Class::NAME) per class.
	classConsts map[string]map[string]any

	// classStatics holds the live value of every `static $name` property, one
	// bag per class. Unlike a constant a static is mutable and shared and the
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
	// The cache holds misses too and a probe does not rescan the method set.
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

	// preludeDone records that the runtime included Options.Include and it
	// installs the declarations that file carries once and redeclares none.
	// A runtime serves one request and this is once per request; ResetSession
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
// entry retains one closure per registered function and the list stays small;
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

// slots returns the pooled slot buffer sized to n. releaseEnv cleared it and
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

// Output returns the writer script output goes to, which under an active
// PushOutput is the innermost one and passes nothing on.
func (rt *Runtime) Output() io.Writer {
	if n := len(rt.outStack); n > 0 {
		return rt.outStack[n-1]
	}
	return rt.out
}

// emptyStdin is the input stream a runtime with no stdin of its own reads:
// no bytes, from any offset.
//
// One value, shared by every runtime and every reset, because it holds no state
// to share. A strings.Reader over "" carries a read offset, which makes it a
// value per use; this carries no state, and a zero-size value costs no allocation
// to put in an interface. The two readers of Runtime.Stdin - STDIN in
// stdlib/core and php://input in stdlib/files - take it as an io.Reader and ask
// no more of it.
type emptyStdin struct{}

func (emptyStdin) Read([]byte) (int, error) { return 0, io.EOF }

// New presizes each table to the size the stdlib fills it to.
//
// A table starting empty rehashes its way up: `phpscript server` builds a
// runtime and registers the whole stdlib into it per request, and over half the
// time inside mapassign on that path went to rehash. The numbers are what the
// `info` command reports as Internal Functions, Declared Classes and Defined
// Constants, rounded up to leave room for what a host and a script add. They
// are a hint and nothing reads them back and one that drifts low costs a rehash
// and no more.
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
// and a tenant cannot read the operator's connection strings.
const InfrastructurePrefix = "PLATFORM_"

// ScriptEnvironment returns the environment scripts read with getenv().
//
// A nil environment means the process environment, as a CLI run has it. A host
// that configured one passes it instead, and a virtual host always does and
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
// registers its bindings and ResetSession can drop what a script defined
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

// Reset returns the runtime to the state a new one is in and a pool can hand
// the same value to the next program, with no second one built.
//
// Reset is ResetSession plus what outlives a session: the parse caches and the
// two maps keyed by AST node, which ResetSession keeps because a --count loop
// runs one program and the keys are the same nodes each time. Across two
// programs they hold the previous tree alive. Reset clears the maps and never
// replaces them and the next program starts on the buckets they grew.
func (rt *Runtime) Reset(out io.Writer, stdin io.Reader) {
	rt.ResetSession(out, stdin)
	clear(rt.compiled)
	clear(rt.sourceSpans)
	rt.includeCache.Clear()
	rt.exprCache.Clear()
	// The tree a lookup indexed went with the include cache and the next one
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

// FS returns the configured source root (or nil).
func (rt *Runtime) FS() fs.FS { return rt.opts.RootFS }

// Stdin returns the input stream configured by the runtime host.
func (rt *Runtime) Stdin() io.Reader { return rt.opts.Stdin }

// WorkDir returns the working directory as an fs.FS path: "." for the root.
func (rt *Runtime) WorkDir() string { return rt.opts.WorkDir }

// WorkDirPath returns the working directory as getcwd() answers it.
func (rt *Runtime) WorkDirPath() string { return rootPath(rt.opts.WorkDir) }

// ResolvePath maps a script's path onto the source filesystem.
func (rt *Runtime) ResolvePath(p string) string { return rt.resolveFSPath(p) }

// SetWorkDir moves the working directory and reports whether it could. A
// relative path resolves against the current one and an absolute path against
// the source root, and neither climbs out of it.
//
// The directory is per-runtime state and a script moving it moves nothing
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

// Database returns the provider named connections resolve through, or nil.
func (rt *Runtime) Database() model.DatabaseProvider { return rt.opts.Database }

// Mail returns the provider named mail servers resolve through, or nil.
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

// IncludeFile evaluates path as PHP in a fresh scope, as `include` would.
func (rt *Runtime) IncludeFile(path string) (any, error) {
	return rt.includeFile(path, false, rt.newScope())
}
