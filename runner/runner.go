package runner

import (
	"fmt"
	"io"
	"path"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/telemetry"
)

// This file is the statement interpreter. expr-lang evaluates expressions, but
// PHP programs are statements: control flow and mutation. The interpreter walks
// the model.Stmt tree, owns the mutable Scope, and calls Runtime.Eval for every
// leaf expression value.

// flow signals non-local control transfer out of a statement list.
type flow int

const (
	flowNormal flow = iota
	flowReturn
	flowBreak
	flowContinue
)

// Run executes a whole program in the global scope.
func (rt *Runtime) Run(p *model.Program) (err error) {
	// The limit belonged to the program that set it. Left armed, it ends the
	// next program run on this runtime, which set none.
	defer rt.resetLimits()
	defer func() {
		err = combineErrors(err, rt.runShutdown())
		if err == nil {
			return
		}
		// exit(0) is how a PHP page ends early, not how it fails. The state
		// feeds the recorded SLA, so only a non-zero code is an error.
		if exit, ok := IsExit(err); ok {
			if exit.Code != 0 {
				rt.UpdateStatus(telemetry.StateError)
			}
			return
		}
		rt.recordTraceError(err)
	}()
	rt.UpdateStatus(telemetry.StateProcessing)
	if err = rt.runPrelude(); err != nil {
		return err
	}
	// Coverage forces the interpreter: flatstack carries no coverage support,
	// and the fallback is atomic, so the whole program runs where exec counts it.
	if rt.flat && rt.coverage == nil {
		if handled, flatErr := rt.runFlat(p); handled {
			err = flatErr
			return err
		}
	}
	err = rt.runInterpreted(p)
	return err
}

// HasErrorHandler reports whether a host installed one with OnError.
func (rt *Runtime) HasErrorHandler() bool { return rt.errorHandler != nil }

// RecordError reports err on the trace of the request this runtime serves,
// unwinding nothing, so no try/catch in the script sees it.
func (rt *Runtime) RecordError(err error) {
	if err == nil {
		return
	}
	rt.recordTraceError(err)
	if rt.errorHandler != nil {
		rt.errorHandler(err)
	}
}

// recordTraceError is the trace half of RecordError, and the whole of what Run
// does with the error a script ended with: that one is already on its way to
// the host as a return value, so routing it to the error handler as well would
// report it twice.
func (rt *Runtime) recordTraceError(err error) {
	rt.UpdateStatus(telemetry.StateError)
	ctx := rt.ctx
	if rt.entrypoint != "" {
		ctx = telemetry.WithSpanFilename(ctx, rt.entrypoint)
	}
	ctx = telemetry.WithSpanLine(ctx, rt.currentSourceLine())
	// The span takes its name from what happened, not from this instance of it:
	// the message is the recorded error, which the front end renders under the
	// span and filters on. A message in the name would make every failure its
	// own kind of failure.
	rt.traceContext(ctx, "php error").RecordError(err)
}

func (rt *Runtime) runInterpreted(p *model.Program) error {
	rt.addSourceSpans(p)
	if rt.coverage != nil {
		rt.coverage.Register(rt.entrypoint, p)
	}
	scope := rt.newScope()
	rt.pushFrame(scope)
	defer rt.popFrame()
	for name, val := range rt.globals {
		scope.Set(name, val)
	}
	rt.auto.Range(func(name string, val any) {
		scope.Set(name, val)
	})
	if rt.entrypoint != "" {
		rt.setScopeFile(scope, rt.entrypoint)
	}
	// Hoist declarations so functions/classes are callable before their textual
	// position (PHP semantics for top-level function/class definitions).
	if err := rt.hoistOnce(p, rt.entrypoint); err != nil {
		return err
	}
	_, _, runErr := rt.exec(p.Stmts, scope)
	return combineErrors(runErr, rt.runDeferred(scope, 0))
}

// hoistOnce hoists a top-level program the first time this runtime sees it
// and is a no-op after. Re-running a compiled program on one runtime is the
// embedding reuse pattern, not PHP's redeclaration, which a single program
// cannot express at this seam; includes keep calling hoist directly, so a
// file included twice still raises the error PHP raises.
func (rt *Runtime) hoistOnce(p *model.Program, filename string) error {
	if _, done := rt.hoisted[p]; done {
		return nil
	}
	if err := rt.hoist(p, filename); err != nil {
		return err
	}
	rt.hoisted[p] = filename
	return nil
}

// hoist registers all function and class declarations found at the given level.
//
// model.CheckInterfaceContracts holds a class declaring `implements` to the
// contract first: the class itself must declare every method the listed
// interfaces name. The check is a name comparison over the AST, so it happens
// before anything runs and copies no member onto the class either way. The
// bytecode backend runs the same check where it collects classes, so both
// raise the same RuntimeException.
// hoist registers an anonymous class here too. The parser reads its
// declaration inside an expression and not as a statement, so it collects the
// class on the program; from there on the class is ordinary, under the name the
// parser gave it.
func (rt *Runtime) hoist(prog *model.Program, filename string) error {
	stmts := prog.Stmts
	if err := model.CheckInterfaceContracts(prog); err != nil {
		return NewRuntimeException(err.Error(), 0)
	}
	// First pass: classes, so a method has a class to attach to.
	classes := map[string]*model.Class{}
	declare := func(cd *model.ClassDecl) {
		c := &model.Class{Name: cd.Name, Implements: model.InterfaceNames(cd, stmts), Fields: cd.Fields, Statics: cd.Statics, Consts: cd.Consts, Methods: map[string]*model.FuncDecl{}}
		for _, m := range cd.Methods {
			// Only filled in, never overwritten: the parser names the file it
			// read the declaration from, and a program parsed without a name
			// takes the runtime's entrypoint instead. Assigning unconditionally
			// made two runtimes hoisting one cached program write the same AST
			// node at once, which is the same rule flatHost.RegisterClass
			// already follows.
			if m.Filename == "" {
				m.Filename = filename
			}
			c.Methods[m.Name] = m
		}
		classes[cd.Name] = c
		rt.RegisterClass(c)
	}
	for _, s := range stmts {
		if cd, ok := s.(*model.ClassDecl); ok {
			declare(cd)
		}
		// An interface registers under its own name so Interface::CONST
		// resolves. It stays out of the class table: it has no constructor,
		// no storage, and confers nothing on an implementing class.
		if id, ok := s.(*model.InterfaceDecl); ok {
			rt.interfaces[id.Name] = id
		}
	}
	for _, cd := range prog.AnonClasses {
		declare(cd)
	}
	// Second pass: functions (free and the `function Class::method` form).
	for _, s := range stmts {
		fd, ok := s.(*model.FuncDecl)
		if !ok {
			continue
		}
		if fd.Filename == "" {
			fd.Filename = filename
		}
		if fd.Class != "" {
			c, ok := classes[fd.Class]
			if !ok {
				return fmt.Errorf("method %s::%s: unknown class", fd.Class, fd.Name)
			}
			c.Methods[fd.Name] = fd
			continue
		}
		if err := rt.declareFunc(fd.Name, declSite(prog, s, filename)); err != nil {
			return err
		}
		decl := fd
		rt.registerUserFunc(fd.Name, func(args ...any) (any, error) {
			return rt.invokeFunc(decl, args)
		})
	}
	return nil
}

// exec runs a statement list, propagating return flow. The statement running
// when the list began is current again once it ends: a call inside an
// expression returns into the statement that made it, and a span the rest of
// that statement starts belongs to its line, not to the callee's last one. An
// error leaves the statement that failed current, for the span reporting it.
func (rt *Runtime) exec(stmts []model.Stmt, scope *Scope) (any, flow, error) {
	outer := rt.currentStmt
	// On entry as well as per statement. A loop body can be empty - `while
	// (true) {}` is the shortest way to write one - and a check that only ran
	// per statement would never run for it, so the limit would not hold.
	if rt.deadlineArmed {
		if err := rt.checkDeadline(); err != nil {
			return nil, flowNormal, err
		}
	}
	for _, s := range stmts {
		// Every statement, because the check is an atomic load off a flag a
		// context armed: a script that slept past its limit and then does
		// three more things has to stop at the first of them. A run with no
		// limit and no request pays two bool loads it never enters.
		// Read per statement and not once on entry: a set_time_limit call
		// halfway down a statement list has to bound the rest of that list.
		if rt.deadlineArmed {
			if err := rt.checkDeadline(); err != nil {
				return nil, flowNormal, err
			}
		}
		// Memory is a walk of the live values, so it stays on a counter.
		if rt.opts.MemoryLimit > 0 {
			if rt.memTick++; rt.memTick >= memCheckStatements {
				rt.memTick = 0
				if err := rt.checkMemory(); err != nil {
					// Returned directly, not through the errorHandler
					// path below: exhaustion must unwind the frame, while an
					// enclosing try still catches it in execTry.
					return nil, flowNormal, err
				}
			}
		}
		rt.currentStmt = s
		if rt.coverage != nil {
			rt.coverage.Hit(s)
		}
		rt.UpdateStatus(telemetry.StateProcessing)
		val, fl, err := rt.execOne(s, scope)
		if err != nil {
			if _, ok := IsExit(err); ok {
				return nil, flowNormal, err
			}
			if rt.errorHandler != nil {
				rt.errorHandler(err)
				continue
			}
			return nil, flowNormal, err
		}
		if fl != flowNormal {
			rt.currentStmt = outer
			return val, fl, nil
		}
	}
	rt.currentStmt = outer
	return nil, flowNormal, nil
}

func (rt *Runtime) execOne(s model.Stmt, scope *Scope) (any, flow, error) {
	switch n := s.(type) {
	case *model.InlineHTML:
		rt.UpdateStatus(telemetry.StateWriting)
		_, err := io.WriteString(rt.Output(), n.Text)
		return nil, flowNormal, err

	case *model.Echo:
		for _, a := range n.Args {
			v, err := rt.Eval(a, scope)
			if err != nil {
				return nil, flowNormal, err
			}
			rt.UpdateStatus(telemetry.StateWriting)
			if _, err := io.WriteString(rt.Output(), phpString(v)); err != nil {
				return nil, flowNormal, err
			}
		}
		return nil, flowNormal, nil

	case *model.ExprStmt:
		_, err := rt.Eval(n.X, scope)
		return nil, flowNormal, err

	case *model.Assign:
		return nil, flowNormal, rt.execAssign(n, scope)

	case *model.If:
		cond, err := rt.Eval(n.Cond, scope)
		if err != nil {
			return nil, flowNormal, err
		}
		if phpTruthy(cond) {
			return rt.exec(n.Then, scope)
		}
		return rt.exec(n.Else, scope)

	case *model.Foreach:
		return rt.execForeach(n, scope)

	case *model.For:
		return rt.execFor(n, scope)

	case *model.DoWhile:
		return rt.execDoWhile(n, scope)

	case *model.Return:
		if n.Value == nil {
			return nil, flowReturn, nil
		}
		v, err := rt.Eval(n.Value, scope)
		return v, flowReturn, err

	case *model.Include:
		_, err := rt.evalInclude(n, scope)
		return nil, flowNormal, err

	case *model.Try:
		return rt.execTry(n, scope)

	case *model.Switch:
		return rt.execSwitch(n, scope)

	case *model.StaticVar:
		return nil, flowNormal, rt.execStaticVar(n, scope)

	case *model.ConstDecl:
		// Each entry declares through the same table define() writes to, so
		// the two spellings of a global constant behave identically.
		for _, c := range n.Consts {
			v, err := rt.Eval(c.Default, scope)
			if err != nil {
				return nil, flowNormal, err
			}
			rt.SetConst(c.Name, v)
		}
		return nil, flowNormal, nil

	case *model.Global:
		// A documented no-op: the variable stays unset (docs/design.md), and
		// `phpscript lint` reports the statement.
		return nil, flowNormal, nil

	case *model.Break:
		return nil, flowBreak, nil

	case *model.Continue:
		return nil, flowContinue, nil

	case *model.Unset:
		return nil, flowNormal, rt.execUnset(n, scope)

	case *model.Throw:
		v, err := rt.Eval(n.X, scope)
		if err != nil {
			return nil, flowNormal, err
		}
		// A built-in throwable is an error already, so it propagates as
		// itself and a catch binds the object a script threw and not a
		// rendering of it.
		if thrown, ok := v.(error); ok {
			return nil, flowNormal, thrown
		}
		// An instance of a declared class is not an error, so it travels
		// wrapped. The wrapper carries its declared class, which is
		// what a clause filters on and what get_class() reports.
		if obj, ok := v.(*model.Object); ok {
			return nil, flowNormal, newObjectError(obj)
		}
		return nil, flowNormal, fmt.Errorf("uncaught exception: %s", phpString(v))

	case *model.Use:
		// The parser resolves imports; the node exists so the formatter
		// can print the statement back out.
		return nil, flowNormal, nil

	case *model.Declare:
		// No directive changes how the runtime behaves, but the block form
		// wraps ordinary code, which still runs.
		return rt.exec(n.Body, scope)

	case *model.FuncDecl, *model.ClassDecl:
		// Already handled by hoist.
		return nil, flowNormal, nil

	case *model.InterfaceDecl:
		// An interface is a contract checked by hoist, not a value: it declares
		// no storage and no body, so there is nothing to register or run.
		return nil, flowNormal, nil

	default:
		return nil, flowNormal, fmt.Errorf("exec: unsupported statement %T", s)
	}
}

// noopTrace is the span closer trace returns when rt.observers is empty. It
// captures nothing, so returning it costs no allocation.
func noopTrace() {}

// trace measures one region of interpreter work: a call, an include, a
// template. The returned closer ends the span, so callers defer it where the
// region begins.
func (rt *Runtime) trace(scope *Scope, message string, kind ...telemetry.Kind) func() {
	if len(rt.observers) == 0 {
		return noopTrace
	}
	span := rt.traceContext(rt.contextWithScope(rt.ctx, scope), message, kind...)
	if span == nil {
		return noopTrace
	}

	// Spans started while the region runs nest below it, so a
	// flat list of calls and includes into the shape of the request.
	restore := rt.ctx
	rt.ctx = span.Context(rt.ctx)
	return func() {
		rt.ctx = restore
		span.End()
	}
}

func (rt *Runtime) addSourceSpans(program *model.Program) {
	for statement, source := range program.SourceSpans {
		rt.sourceSpans[statement] = source
	}
}

// currentSourceLine answers the line rt.currentStmt starts on, or zero for a
// statement the parser recorded no span for - one the runner synthesised,
// which no source line names.
func (rt *Runtime) currentSourceLine() int {
	if rt.currentStmt == nil {
		return 0
	}
	return rt.sourceSpans[rt.currentStmt].Start
}

// setScopeFile binds __FILE__ and __DIR__ into a fresh call frame. The frame
// is new, so the bind shadows nothing and needs no restore; includeFile,
// which layers onto a live scope, resolves through pushScopeFile.
func (rt *Runtime) setScopeFile(scope *Scope, filename string) {
	scope.Set("__FILE__", rt.boxed(filename))
	scope.Set("__DIR__", rt.boxed(rt.fileDir(filename)))
}

// boxed answers s as an interface value, boxing each distinct string once.
//
// A string does not fit in an interface word, so every `any(s)` is a heap
// cell. The strings this covers are the magic constants and the class name a
// frame binds, which come from the declarations a program holds: the set is
// the size of the source and every call binds the same members of it.
//
// The map is the runtime's, like fileDirs beside it, because a Runtime answers
// one goroutine at a time and Fork builds a worker its own. A cache on the
// declaration would travel through the include cache, where two workers
// reach one AST.
func (rt *Runtime) boxed(s string) any {
	if v, ok := rt.boxedStrings[s]; ok {
		return v
	}
	if rt.boxedStrings == nil {
		rt.boxedStrings = make(map[string]any, 16)
	}
	var v any = s
	rt.boxedStrings[s] = v
	return v
}

// fileDir caches path.Dir per filename: every invocation of a function binds
// its declaring file's directory, and the set of filenames is the set of
// loaded scripts, so the split happens once per file and not once per call.
func (rt *Runtime) fileDir(filename string) string {
	if d, ok := rt.fileDirs[filename]; ok {
		return d
	}
	d := path.Dir(filename)
	if rt.fileDirs == nil {
		rt.fileDirs = make(map[string]string, 8)
	}
	rt.fileDirs[filename] = d
	return d
}

// scopeFileState is what pushScopeFile shadowed; a value and not a
// closure, so the caller's deferred restore stays on the stack.
type scopeFileState struct {
	file, dir       any
	hadFile, hadDir bool
}

// pushScopeFile binds __FILE__ and __DIR__ over a live scope, remembering
// what an enclosing file had bound.
func (rt *Runtime) pushScopeFile(scope *Scope, filename string) scopeFileState {
	s := scopeFileState{}
	s.file, s.hadFile = scope.Get("__FILE__")
	s.dir, s.hadDir = scope.Get("__DIR__")
	rt.setScopeFile(scope, filename)
	return s
}

func (s scopeFileState) restore(scope *Scope) {
	if s.hadFile {
		scope.Set("__FILE__", s.file)
	} else {
		delete(scope.vars, "__FILE__")
	}
	if s.hadDir {
		scope.Set("__DIR__", s.dir)
	} else {
		delete(scope.vars, "__DIR__")
	}
}
