package runner

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
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

// IncludeFunc resolves an include/require path to a parsed program. Wiring this
// from the host keeps the runner free of file-system and parser dependencies.
type IncludeFunc func(path string) (*model.Program, error)

// SetIncludeResolver installs the include/require resolver.
func (rt *Runtime) SetIncludeResolver(fn IncludeFunc) { rt.include = fn }

// RegisterInclude installs a host implementation of one include target: an
// include or require of path runs fn instead of parsing the file, and the
// script sees fn's return value where PHP would see the file's.
//
// This is for files the runtime reimplements in Go. composer's generated
// vendor/autoload.php is the motivating case: it bootstraps a class loader
// through PHP features the interpreter does not support, while phpscript can
// read the same composer metadata natively. Binding it here rather than
// installing the loader at startup keeps the PHP semantics intact: nothing is
// autoloadable until the script has actually included the autoloader.
func (rt *Runtime) RegisterInclude(path string, fn func() (any, error)) {
	if rt.includeHooks == nil {
		rt.includeHooks = map[string]func() (any, error){}
	}
	rt.includeHooks[cleanFSPath(path)] = fn
}

// Load parses PHP source into a program.
func (rt *Runtime) Load(src string) (*model.Program, error) {
	rt.UpdateStatus(telemetry.StateReading)
	program, err := parser.Parse(src)
	if err != nil {
		rt.UpdateStatus(telemetry.StateError)
	}
	return program, err
}

// LoadFile reads and parses a PHP file from the runtime source FS.
func (rt *Runtime) LoadFile(path string) (*model.Program, error) {
	rt.UpdateFilename(path)
	rt.UpdateStatus(telemetry.StateReading)
	if rt.opts.RootFS == nil {
		rt.UpdateStatus(telemetry.StateError)
		return nil, fmt.Errorf("load %q: no source FS configured", path)
	}
	return rt.loadResolved(rt.resolveFSPath(path))
}

// loadResolved reads and parses a path the caller has already put through
// resolveFSPath. Resolution happens once per include, above the cache lookup,
// so that the cache is keyed by the file rather than by the spelling: after a
// chdir the same "x.php" names a different file, and a cache keyed on the
// spelling would answer with the previous one.
func (rt *Runtime) loadResolved(cleanPath string) (*model.Program, error) {
	b, err := fs.ReadFile(rt.opts.RootFS, cleanPath)
	if err != nil {
		rt.UpdateStatus(telemetry.StateError)
		return nil, fmt.Errorf("load %q: %w", cleanPath, err)
	}
	prog, err := rt.Load(string(b))
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", cleanPath, err)
	}
	return prog, nil
}

// Run executes a whole program in the global scope.
func (rt *Runtime) Run(p *model.Program) (err error) {
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
	// Coverage forces the interpreter: flatstack carries no coverage support,
	// and the fallback is atomic, so the whole program runs where it is counted.
	if rt.flat && rt.coverage == nil {
		if handled, flatErr := rt.runFlat(p); handled {
			err = flatErr
			return err
		}
	}
	err = rt.runInterpreted(p)
	return err
}

// RecordError reports err on the trace of the request this runtime is serving,
// as the failure of the script it is running. Run calls it for the error a
// script ends with; a host calls it for a failure that happened outside the
// script, before or around it, which the script therefore has no throw to
// catch: a request body refused for its size, say.
//
// It records and returns. It does not unwind PHP execution, so nothing about it
// is visible to a script through try/catch; a Go host observes it on the trace,
// or through the handler installed with OnError.
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
	ctx = telemetry.WithSpanLine(ctx, rt.currentLine)
	// The span is named for what happened, not for this instance of it: the
	// message is the recorded error, which the front end renders under the
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
	if rt.entrypoint != "" {
		setScopeFile(scope, rt.entrypoint)
	}
	// Hoist declarations so functions/classes are callable before their textual
	// position (PHP semantics for top-level function/class definitions).
	if err := rt.hoist(p, rt.entrypoint); err != nil {
		return err
	}
	_, _, runErr := rt.exec(p.Stmts, scope)
	return combineErrors(runErr, rt.runDeferred(scope, 0))
}

// hoist registers all function and class declarations found at the given level.
//
// A class declaring `implements` is held to the contract first: every method
// the listed interfaces name must be declared by the class itself. The check is
// a name comparison over the AST, so it happens before anything runs and no
// member is copied onto the class either way. The bytecode backend runs the
// same check where it collects classes, so both raise the same
// RuntimeException.
// An anonymous class is registered here too. Its declaration is written inside
// an expression rather than as a statement, so the parser collects it on the
// program; from this point on it is an ordinary class under the name the parser
// gave it.
func (rt *Runtime) hoist(prog *model.Program, filename string) error {
	stmts := prog.Stmts
	if err := model.CheckInterfaceContracts(prog); err != nil {
		return NewRuntimeException(err.Error(), 0)
	}
	// First pass: classes, so methods can be attached.
	classes := map[string]*model.Class{}
	declare := func(cd *model.ClassDecl) {
		c := &model.Class{Name: cd.Name, Implements: model.InterfaceNames(cd, stmts), Fields: cd.Fields, Statics: cd.Statics, Consts: cd.Consts, Methods: map[string]*model.FuncDecl{}}
		for _, m := range cd.Methods {
			m.Filename = filename
			c.Methods[m.Name] = m
		}
		classes[cd.Name] = c
		rt.RegisterClass(c)
	}
	for _, s := range stmts {
		if cd, ok := s.(*model.ClassDecl); ok {
			declare(cd)
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
		fd.Filename = filename
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

// exec runs a statement list, propagating return flow.
func (rt *Runtime) exec(stmts []model.Stmt, scope *Scope) (any, flow, error) {
	for _, s := range stmts {
		if rt.opts.MemoryLimit > 0 {
			if rt.memTick++; rt.memTick >= memCheckStatements {
				rt.memTick = 0
				if err := rt.checkMemory(); err != nil {
					// Returned directly rather than through the errorHandler
					// path below: exhaustion must unwind the frame, while an
					// enclosing try still catches it in execTry.
					return nil, flowNormal, err
				}
			}
		}
		if source, ok := rt.sourceSpans[s]; ok {
			rt.currentLine = source.Start
			scope.Set("__LINE__", source.Start)
		}
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
			return val, fl, nil
		}
	}
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
		// itself and a catch binds the object a script threw rather than a
		// rendering of it.
		if thrown, ok := v.(error); ok {
			return nil, flowNormal, thrown
		}
		// An instance of a declared class is not an error, so it travels
		// wrapped. The wrapper carries the class it was declared as, which is
		// what a clause filters on and what get_class() reports.
		if obj, ok := v.(*model.Object); ok {
			return nil, flowNormal, newObjectError(obj)
		}
		return nil, flowNormal, fmt.Errorf("uncaught exception: %s", phpString(v))

	case *model.Use:
		// Imports are resolved while parsing; the node exists so the
		// formatter can print the statement back out.
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

func (rt *Runtime) execForeach(n *model.Foreach, scope *Scope) (any, flow, error) {
	src, err := rt.Eval(n.Source, scope)
	if err != nil {
		return nil, flowNormal, err
	}
	keyTarget, valTarget := n.KeyTarget, n.ValTarget
	if keyTarget == nil && n.KeyVar != "" {
		keyTarget = &model.Var{Name: n.KeyVar}
	}
	if valTarget == nil && n.ValVar != "" {
		valTarget = &model.Var{Name: n.ValVar}
	}
	var (
		val   any
		ret   flow
		retOK bool
	)
	// PHP's two loop semantics. `as &$v` binds the element, so a body that
	// assigns to the target edits the source; `as $v` binds a copy, so it does
	// not. Only a *model.Array can be written back to or copied; a collection
	// a binding returned belongs to the host rather than to the script.
	//
	// The copy is made only when the body actually assigns through the target,
	// because phpscript has no refcount to defer it with: an unconditional copy
	// would charge every loop for a semantic almost none of them use. The test
	// runs once per loop, not once per iteration.
	var writable *model.Array
	if n.ByRef {
		writable, _ = src.(*model.Array)
	}
	copyValue := !n.ByRef && model.AssignsTo(n.Body, valTarget)

	// iter runs the loop body for one (key, value) pair, returning whether to
	// continue iterating. It records any error / non-normal flow in the
	// enclosing variables.
	iter := func(k, v any) bool {
		if keyTarget != nil {
			if err = rt.assignTo(keyTarget, k, scope); err != nil {
				return false
			}
		}
		if copyValue {
			v = model.CopyValue(v)
		}
		if err = rt.assignTo(valTarget, v, scope); err != nil {
			return false
		}
		var fl flow
		val, fl, err = rt.exec(n.Body, scope)
		if err != nil {
			return false
		}
		// The write-back happens before the flow is inspected: a reference makes
		// the element live, so a body that assigns and then breaks or returns
		// has still edited the source.
		if writable != nil {
			if updated, readErr := rt.readLValue(valTarget, scope); readErr == nil {
				writable.Set(k, updated)
			}
		}
		switch fl {
		case flowReturn:
			ret, retOK = flowReturn, true
			return false
		case flowBreak:
			return false
		case flowContinue:
			return true
		}
		return true
	}

	switch src := src.(type) {
	case *model.Array:
		src.Range(iter)
	case *model.Object:
		// An object yields its properties, name and value, in the order it
		// reads them back. PHP yields only the ones visible where the loop is
		// written; phpscript enforces no visibility anywhere, so it yields all
		// of them. See docs/README.md.
		src.Range(func(name string, value any) bool {
			return iter(name, value)
		})
	default:
		// Native Go collections (e.g. a []Record returned by a forwarded
		// method) are iterable too: slices/arrays by integer index, maps by key.
		rv := reflect.ValueOf(src)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				if !iter(int64(i), rv.Index(i).Interface()) {
					break
				}
			}
		case reflect.Map:
			for _, mk := range rv.MapKeys() {
				if !iter(mk.Interface(), rv.MapIndex(mk).Interface()) {
					break
				}
			}
		}
		// Any other non-iterable source is skipped (PHP warns and continues).
	}

	if err != nil {
		return nil, flowNormal, err
	}
	if retOK {
		return val, ret, nil
	}
	return nil, flowNormal, nil
}

func (rt *Runtime) execFor(n *model.For, scope *Scope) (any, flow, error) {
	if n.Init != nil {
		if _, _, err := rt.execOne(n.Init, scope); err != nil {
			return nil, flowNormal, err
		}
	}
	for {
		if n.Cond != nil {
			c, err := rt.Eval(n.Cond, scope)
			if err != nil {
				return nil, flowNormal, err
			}
			if !phpTruthy(c) {
				break
			}
		}
		val, fl, err := rt.exec(n.Body, scope)
		if err != nil {
			return nil, flowNormal, err
		}
		if fl == flowReturn {
			return val, fl, nil
		}
		if fl == flowBreak {
			break
		}
		// flowContinue / flowNormal both fall through to the post step.
		if n.Post != nil {
			if _, _, err := rt.execOne(n.Post, scope); err != nil {
				return nil, flowNormal, err
			}
		}
	}
	return nil, flowNormal, nil
}

// execDoWhile runs the body before the first condition check, so it executes
// at least once. continue falls through to the check, matching PHP.
func (rt *Runtime) execDoWhile(n *model.DoWhile, scope *Scope) (any, flow, error) {
	for {
		val, fl, err := rt.exec(n.Body, scope)
		if err != nil {
			return nil, flowNormal, err
		}
		if fl == flowReturn {
			return val, fl, nil
		}
		if fl == flowBreak {
			break
		}
		c, err := rt.Eval(n.Cond, scope)
		if err != nil {
			return nil, flowNormal, err
		}
		if !phpTruthy(c) {
			break
		}
	}
	return nil, flowNormal, nil
}

// execTry runs the try body; if it raises an error (a throw or a runtime error
// from a forwarded Go call), the matching catch clause handles it with the error
// bound to its variable (so `echo $e` prints the message). A finally block, if
// present, always runs.
//
// exit() and die() are not errors and are not catchable, which is what PHP
// does: `try { exit(); } catch (Throwable $e) {}` ends the script there. They
// travel as an error here only because that is how the interpreter unwinds, so
// the try has to recognise the sentinel and get out of the way. A catch that
// could swallow an exit would turn `header("Location: ...") ; exit();` inside a
// try into a page that carries on running after the redirect was staged.
func (rt *Runtime) execTry(n *model.Try, scope *Scope) (any, flow, error) {
	val, fl, err := rt.exec(n.Body, scope)
	if _, exiting := IsExit(err); exiting {
		// finally is skipped too: PHP runs no finally block on exit.
		return val, fl, err
	}

	if err != nil && len(n.Catches) > 0 {
		rootErr := err
		for {
			if unwrapped := errors.Unwrap(rootErr); unwrapped != nil {
				rootErr = unwrapped
				continue
			}
			break
		}

		for _, c := range n.Catches {
			if matchCatchType(c.Type, rootErr) {
				if c.Var != "" {
					scope.Set(c.Var, catchValue(rootErr))
				}
				val, fl, err = rt.exec(c.Body, scope)
				break
			}
		}
	}
	if len(n.Finally) > 0 {
		fVal, fFl, fErr := rt.exec(n.Finally, scope)
		// A finally that returns/throws overrides the try/catch outcome.
		if fErr != nil {
			return fVal, flowNormal, fErr
		}
		if fFl != flowNormal {
			return fVal, fFl, nil
		}
	}
	return val, fl, err
}

// matchCatchType reports whether a catch clause declaring declaredType handles
// rootErr. A union takes the error when any alternative does.
//
// There is no class hierarchy to walk, so the clause is answered from the name
// the throwable records:
//
//   - nothing, or Throwable, takes everything.
//   - an error that is no PHP class at all takes any clause, so a Go binding's
//     error reaches the catch a script already wrote around the call.
//   - Exception takes any class whose name does not end in "Error", and Error
//     takes any class whose name does. That suffix is the whole of the split
//     between a fault in the program and a condition it raised, and it agrees
//     with PHP for every built-in name: ErrorException is an Exception,
//     AssertionError and TypeError are Errors.
//   - any other name is compared to the class, case-insensitively.
func matchCatchType(declaredType string, rootErr error) bool {
	declaredType = strings.TrimSpace(declaredType)
	if declaredType == "" {
		return true
	}

	class, isThrowable := throwableClassOf(rootErr)
	for _, part := range strings.Split(declaredType, "|") {
		want := strings.TrimPrefix(strings.TrimSpace(part), "\\")
		switch {
		case want == "" || want == "Throwable":
			return true
		case !isThrowable:
			return true
		case want == "Exception":
			if !strings.HasSuffix(class, "Error") {
				return true
			}
		case want == "Error":
			if strings.HasSuffix(class, "Error") {
				return true
			}
		case strings.EqualFold(want, class):
			return true
		}
	}
	return false
}

// execSwitch evaluates the discriminant and runs matching case bodies with PHP
// fall-through; break stops the switch, return propagates out.
func (rt *Runtime) execSwitch(n *model.Switch, scope *Scope) (any, flow, error) {
	cond, err := rt.Eval(n.Cond, scope)
	if err != nil {
		return nil, flowNormal, err
	}
	matched := false
	for _, c := range n.Cases {
		if !matched {
			cv, err := rt.Eval(c.Value, scope)
			if err != nil {
				return nil, flowNormal, err
			}
			if !phpLooseEqual(cond, cv) {
				continue
			}
			matched = true
		}
		val, fl, err := rt.exec(c.Body, scope)
		if err != nil {
			return nil, flowNormal, err
		}
		switch fl {
		case flowReturn:
			return val, flowReturn, nil
		case flowBreak:
			return nil, flowNormal, nil
		case flowContinue:
			return nil, flowContinue, nil
		}
	}
	if !matched && n.Default != nil {
		val, fl, err := rt.exec(n.Default, scope)
		if err != nil {
			return nil, flowNormal, err
		}
		switch fl {
		case flowReturn:
			return val, flowReturn, nil
		case flowContinue:
			return nil, flowContinue, nil
		}
	}
	return nil, flowNormal, nil
}

func (rt *Runtime) evalInclude(n *model.Include, scope *Scope) (any, error) {
	path, err := rt.Eval(n.Path, scope)
	if err != nil {
		return nil, err
	}
	return rt.includeFile(phpString(path), n.Once, scope)
}

// noopTrace is the span closer returned when no observer is registered. It
// captures nothing, so returning it does not allocate.
func noopTrace() {}

// trace measures one region of interpreter work: a call, an include, a
// template. The returned closer ends the span, so callers defer it where the
// region begins.
func (rt *Runtime) trace(scope *Scope, message string, kind ...telemetry.Kind) func() {
	if len(rt.observers) == 0 {
		return noopTrace
	}
	span := rt.traceContext(contextWithScope(rt.ctx, scope), message, kind...)
	if span == nil {
		return noopTrace
	}

	// Spans started while the region runs nest below it, which is what turns a
	// flat list of calls and includes into the shape of the request.
	restore := rt.ctx
	rt.ctx = span.Context(rt.ctx)
	return func() {
		rt.ctx = restore
		span.End()
	}
}

// includeFile evaluates one include. once is the *_once form, which is answered
// here rather than at the call site so that both engines dedupe on the same
// thing: the file that was resolved, not the spelling the script used. After a
// chdir two directories can spell one name, and a scan over spellings would
// skip the second file as though it had already run.
func (rt *Runtime) includeFile(path string, once bool, scope *Scope) (any, error) {
	resolved := rt.resolveFSPath(path)
	if once && slices.Contains(rt.included, rootPath(resolved)) {
		return true, nil
	}

	kind := telemetry.KindInternal
	if strings.EqualFold(filepath.Ext(path), ".tpl") {
		kind = telemetry.KindTemplate
	}
	defer rt.trace(scope, "include "+path, kind)()

	if hook, ok := rt.includeHooks[resolved]; ok {
		rt.included = append(rt.included, rootPath(resolved))
		rt.UpdateIncludedFiles(len(rt.included))
		return hook()
	}

	prog, filename, err := rt.resolveInclude(path)
	if err != nil {
		return nil, fmt.Errorf("error including %s: %w", path, err)
	}
	rt.addSourceSpans(prog)
	if rt.coverage != nil {
		rt.coverage.Register(filename, prog)
	}
	rt.included = append(rt.included, rootPath(filename))
	rt.UpdateIncludedFiles(len(rt.included))
	restoreFile := setScopeFile(scope, rootPath(filename))
	defer restoreFile()
	if err := rt.hoist(prog, rootPath(filename)); err != nil {
		return nil, err
	}
	deferMark := len(scope.deferred)
	value, fl, runErr := rt.exec(prog.Stmts, scope)
	err = combineErrors(runErr, rt.runDeferred(scope, deferMark))
	if err != nil {
		return nil, err
	}
	if fl == flowReturn {
		return value, nil
	}
	// PHP include/require constructs evaluate to 1 when the included file
	// reaches its end without an explicit return.
	return int64(1), nil
}

func (rt *Runtime) addSourceSpans(program *model.Program) {
	for statement, source := range program.SourceSpans {
		rt.sourceSpans[statement] = source
	}
}

func setScopeFile(scope *Scope, filename string) func() {
	previousFile, hadFile := scope.Get("__FILE__")
	previousDir, hadDir := scope.Get("__DIR__")
	scope.Set("__FILE__", filename)
	scope.Set("__DIR__", path.Dir(filename))
	return func() {
		if hadFile {
			scope.Set("__FILE__", previousFile)
		} else {
			delete(scope.vars, "__FILE__")
		}
		if hadDir {
			scope.Set("__DIR__", previousDir)
		} else {
			delete(scope.vars, "__DIR__")
		}
	}
}

func (rt *Runtime) autoload(class string, scope *Scope) error {
	for _, loader := range rt.autoloaders {
		// Every PHP callable spelling reaches here: composer registers
		// array($this, "loadClass"), its bootstrap registers the
		// "Class::method" string form, and a plain function name or closure is
		// just as valid.
		callable, ok := rt.callableWithScope(loader, scope)
		if !ok {
			return fmt.Errorf("autoload callback %v is not callable", loader)
		}
		if _, err := callable(class); err != nil {
			return err
		}
		if rt.hasClass(class) {
			return nil
		}
	}
	// The autoload folder is a fallback rather than an entry in the queue, so
	// spl_autoload_functions and spl_autoload_unregister keep answering for what
	// the script registered and a callback still gets first refusal on a class
	// the folder could have answered.
	return rt.folderAutoload(class)
}

// folderAutoload resolves a class against the autoload folder, the convention
// that replaces a bootstrap for a site that has no composer: the namespace is
// the directory path below the folder, case for case, and the class is the
// file. `Acme\Thing` is autoload/Acme/Thing.php, and the namespace is optional,
// so a class declared in none is autoload/Bare.php.
//
// The folder is disabled by not being there. Nothing scans it and nothing is
// loaded ahead of time: this runs on a class reference that was otherwise about
// to fail, so a file nobody names is a file nobody reads.
func (rt *Runtime) folderAutoload(class string) error {
	if rt.opts.RootFS == nil {
		return nil
	}
	segments := autoloadSegments(class)
	if segments == nil {
		return nil
	}
	filename := path.Join(filepath.ToSlash(rt.opts.autoloadDir()), path.Join(segments...)+".php")
	cleanPath := rt.resolveFSPath(filename)
	if _, err := fs.Stat(rt.opts.RootFS, cleanPath); err != nil {
		return nil
	}
	// A file that names the class it was loaded for without declaring it would
	// otherwise include itself until the stack runs out. PHP guards the same
	// way: a class already being autoloaded is not autoloaded again.
	if _, busy := rt.autoloading[cleanPath]; busy {
		return nil
	}
	if rt.autoloading == nil {
		rt.autoloading = map[string]struct{}{}
	}
	rt.autoloading[cleanPath] = struct{}{}
	defer delete(rt.autoloading, cleanPath)

	_, err := rt.includeFile(filename, false, rt.newScope())
	return err
}

// autoloadSegments splits a class name into the path segments the folder
// convention names, or nil when any of them is not a PHP label.
//
// The check is what keeps a class name from being a path. class_exists takes an
// arbitrary string, so "../secret" has to be a miss rather than a file read
// outside the folder.
func autoloadSegments(class string) []string {
	class = strings.TrimPrefix(class, "\\")
	if class == "" {
		return nil
	}
	segments := strings.Split(class, "\\")
	for _, segment := range segments {
		if !isPHPLabel(segment) {
			return nil
		}
	}
	return segments
}

// isPHPLabel reports whether s is spelled the way PHP spells an identifier:
// a letter, underscore or high byte first, then those plus digits.
func isPHPLabel(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r >= 0x80:
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// UnregisterAutoloader removes a callback from the SPL autoload queue, matching
// it the way PHP does: by the function, object and method it names rather than
// by identity, since each `array($this, "loadClass")` is a fresh array.
func (rt *Runtime) UnregisterAutoloader(callback any) bool {
	want := autoloaderKey(callback)
	for i, loader := range rt.autoloaders {
		if autoloaderKey(loader) != want {
			continue
		}
		rt.autoloaders = append(rt.autoloaders[:i], rt.autoloaders[i+1:]...)
		return true
	}
	return false
}

// autoloaderKey renders a registered callback as a comparable identity.
func autoloaderKey(callback any) string {
	switch value := callback.(type) {
	case string:
		return "fn:" + value
	case *model.Array:
		if value.Len() != 2 {
			break
		}
		target, _ := value.Get(int64(0))
		method, _ := value.Get(int64(1))
		if object, ok := target.(*model.Object); ok {
			return fmt.Sprintf("method:%p:%v", object, method)
		}
		return fmt.Sprintf("static:%v:%v", target, method)
	}
	// A closure or a host func has no PHP-visible name, so its code pointer is
	// the only identity available.
	if rv := reflect.ValueOf(callback); rv.IsValid() {
		switch rv.Kind() {
		case reflect.Func, reflect.Pointer, reflect.Map, reflect.Slice, reflect.UnsafePointer:
			return fmt.Sprintf("value:%v", rv.Pointer())
		}
	}
	return fmt.Sprintf("value:%v", callback)
}

// SPLAutoload implements PHP's default autoloader: lowercase the qualified
// class name and search each include_path entry for class.php.
func (rt *Runtime) SPLAutoload(class string) error {
	class = strings.ToLower(strings.TrimPrefix(class, "\\"))
	class = strings.ReplaceAll(class, "\\", "/")
	for _, dir := range filepath.SplitList(rt.includePath) {
		if dir == "" {
			dir = "."
		}
		for _, ext := range []string{".php"} {
			filename := path.Join(filepath.ToSlash(dir), class+ext)
			cleanPath := rt.resolveFSPath(filename)
			if rt.opts.RootFS == nil {
				continue
			}
			if _, err := fs.Stat(rt.opts.RootFS, cleanPath); err != nil {
				continue
			}
			_, err := rt.includeFile(filename, false, rt.newScope())
			return err
		}
	}
	return nil
}

// resolveInclude turns an include path into a parsed program, preferring an
// explicit IncludeFunc resolver and otherwise reading from the configured fs.FS.
func (rt *Runtime) resolveInclude(path string) (*model.Program, string, error) {
	if rt.include != nil {
		prog, err := rt.include(path)
		return prog, cleanFSPath(path), err
	}
	if rt.opts.RootFS != nil {
		cleanPath := rt.resolveFSPath(path)
		if prog, ok := rt.includeCache.Get(cleanPath); ok {
			return prog, cleanPath, nil
		}

		prog, err := rt.loadResolved(cleanPath)
		if err != nil {
			return nil, "", fmt.Errorf("include %q: %w", path, err)
		}
		rt.includeCache.Set(cleanPath, prog)
		return prog, cleanPath, nil
	}
	return nil, "", fmt.Errorf("include %q: no resolver or source FS configured", path)
}

// cleanFSPath normalises an include path for fs.FS, which requires slash-rooted,
// unrooted, dot-free paths.
func cleanFSPath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	return path.Clean(p)
}

// clampFSPath cleans p against the root: a ".." that would climb above it is
// collapsed rather than escaping, so "a/../../etc/passwd" names etc/passwd
// inside the root. Cleaning happens against "/" first, which is what does the
// collapsing; path.Clean on its own keeps a leading "..".
func clampFSPath(p string) string {
	clean := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(p)), "/")
	if clean == "" {
		return "."
	}
	return clean
}

// rootPath spells an fs.FS path the way a script reads it. The source
// filesystem is mounted at "/", so a name below it is written from there, and
// that is what __FILE__, __DIR__ and getcwd() answer with.
//
// The point of the leading slash is that resolveFSPath leaves it alone: a path
// written from the root names one file whatever the working directory is, the
// way an absolute path does in PHP. Without it `include __DIR__ . "/x.php"`
// would be re-resolved against the working directory and name a different file
// after every chdir, which is what composer's autoloader is built on.
func rootPath(p string) string {
	clean := clampFSPath(p)
	if clean == "." {
		return "/"
	}
	return "/" + clean
}

// resolveFSPath maps a path a script supplied onto the source filesystem,
// through the working directory chdir() moved.
//
// A path written from the root is taken as it is; anything else is relative to
// the working directory. Both are cleaned against the root, so neither can
// climb out of it.
func (rt *Runtime) resolveFSPath(p string) string {
	slash := filepath.ToSlash(p)
	if strings.HasPrefix(slash, "/") {
		return clampFSPath(slash)
	}
	if rt.opts.WorkDir == "" || rt.opts.WorkDir == "." {
		return clampFSPath(slash)
	}
	// The working directory is joined before the clamp, not after, so a ".."
	// climbs out of it the way it would on a real filesystem and stops at the
	// root rather than at the directory the script happens to be in.
	return clampFSPath(rt.opts.WorkDir + "/" + slash)
}

// execAssign mutates a variable, property or array element. expr-lang cannot do
// this, so it lives entirely here.
func (rt *Runtime) execAssign(n *model.Assign, scope *Scope) error {
	rhs, err := rt.Eval(n.Value, scope)
	if err != nil {
		return err
	}
	bindConstructorID(n.Target, n.Value, n.Op, rhs)

	switch tgt := model.UnwrapParenthesized(n.Target).(type) {
	case *model.Var:
		cur, _ := scope.Get(tgt.Name)
		next, err := applyAssignOp(n.Op, cur, rhs)
		if err != nil {
			return err
		}
		rt.setVar(scope, tgt.Name, next)
		return nil

	case *model.ListExpr:
		// list($a, $b) = $arr destructures by position (integer keys). The
		// source is read through helperIndex, so a binding returning a native
		// []string destructures like an *Array.
		if !model.IsCollection(rhs) {
			return fmt.Errorf("assign: list() target requires an array")
		}
		for i, el := range tgt.Elems {
			if el == nil {
				continue
			}
			if err := rt.assignTo(el, helperIndex(rhs, int64(i)), scope); err != nil {
				return err
			}
		}
		return nil

	case *model.PropAccess:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return err
		}
		if obj, ok := base.(*model.Object); ok {
			next, err := applyAssignOp(n.Op, obj.Props[tgt.Name], rhs)
			if err != nil {
				return err
			}
			obj.SetProp(tgt.Name, next)
			return nil
		}
		return assignGoField(base, tgt.Name, func(current any) (any, error) {
			return applyAssignOp(n.Op, current, rhs)
		})

	case *model.StaticProp:
		bag, err := rt.staticStorage(tgt.Class, scope)
		if err != nil {
			return err
		}
		next, err := applyAssignOp(n.Op, bag[tgt.Name], rhs)
		if err != nil {
			return err
		}
		bag[tgt.Name] = next
		return nil

	case *model.Index:
		base, err := rt.container(tgt.Base, scope)
		if err != nil {
			return err
		}
		arr, ok := base.(*model.Array)
		if !ok {
			// Native Go collections are writable where Go itself allows it: map
			// entries can be added or replaced, existing slice elements can be
			// overwritten. Only `$a[] =` needs an *Array, since a slice cannot
			// grow through the interface holding it.
			if n.Op == "[]=" || tgt.Index == nil {
				return fmt.Errorf("assign: cannot append to %T; a binding whose result is appended to must return *model.Array", base)
			}
			key, err := rt.Eval(tgt.Index, scope)
			if err != nil {
				return err
			}
			return assignGoIndex(base, key, func(current any) (any, error) {
				return applyAssignOp(n.Op, current, rhs)
			})
		}
		if n.Op == "[]=" || tgt.Index == nil {
			arr.Append(rhs)
			return nil
		}
		key, err := rt.Eval(tgt.Index, scope)
		if err != nil {
			return err
		}
		k := normalizeKey(key)
		cur, _ := arr.Get(k)
		next, err := applyAssignOp(n.Op, cur, rhs)
		if err != nil {
			return err
		}
		arr.Set(k, next)
		return nil

	default:
		return fmt.Errorf("assign: unsupported target %T", n.Target)
	}
}

func bindConstructorID(target, value model.Expr, op string, result any) {
	if op != "" && op != "=" {
		return
	}
	variable, ok := model.UnwrapParenthesized(target).(*model.Var)
	if !ok {
		return
	}
	if _, ok := model.UnwrapParenthesized(value).(*model.New); !ok {
		return
	}
	if identifiable, ok := result.(interface{ SetID(string) }); ok {
		identifiable.SetID(variable.Name)
	}
}

// container evaluates the base of an index assignment, creating the array PHP
// would create when nothing is there yet. `$rows[$first][$second] = $v` against
// an unset $rows makes both levels; the new array has to be written back into
// the slot that held null, or the write would land in a value nothing holds.
//
// Only an assignable base is auto-vivified. A function result or a literal has
// nowhere to write the array back to, so it is returned as-is and the caller
// reports the assignment as unsupported.
func (rt *Runtime) container(base model.Expr, scope *Scope) (any, error) {
	value, err := rt.Eval(base, scope)
	if err != nil {
		return nil, err
	}
	if value != nil || !assignable(base) {
		return value, nil
	}
	created := model.NewArray()
	if err := rt.assignTo(base, created, scope); err != nil {
		return nil, err
	}
	return created, nil
}

// assignable reports whether e names storage that can be written back to.
func assignable(e model.Expr) bool {
	switch model.UnwrapParenthesized(e).(type) {
	case *model.Var, *model.Index, *model.PropAccess, *model.StaticProp:
		return true
	}
	return false
}

// execUnset removes each target from the scope, array, property bag or static
// bag holding it. Unsetting something that was never set is not an error, which
// is what lets `unset($map[$key])` run unconditionally.
func (rt *Runtime) execUnset(n *model.Unset, scope *Scope) error {
	for _, target := range n.Targets {
		switch tgt := model.UnwrapParenthesized(target).(type) {
		case *model.Var:
			scope.Unset(tgt.Name)
		case *model.PropAccess:
			base, err := rt.Eval(tgt.Base, scope)
			if err != nil {
				return err
			}
			if obj, ok := base.(*model.Object); ok {
				obj.DeleteProp(tgt.Name)
			}
		case *model.StaticProp:
			bag, err := rt.staticStorage(tgt.Class, scope)
			if err != nil {
				return err
			}
			delete(bag, tgt.Name)
		case *model.Index:
			if tgt.Index == nil {
				return fmt.Errorf("unset: [] is not an unset target")
			}
			base, err := rt.Eval(tgt.Base, scope)
			if err != nil {
				return err
			}
			key, err := rt.Eval(tgt.Index, scope)
			if err != nil {
				return err
			}
			if arr, ok := base.(*model.Array); ok {
				arr.Delete(normalizeKey(key))
			}
		default:
			return fmt.Errorf("unset: unsupported target %T", target)
		}
	}
	return nil
}

func (rt *Runtime) readLValue(target model.Expr, scope *Scope) (any, error) {
	switch tgt := model.UnwrapParenthesized(target).(type) {
	case *model.Var:
		v, _ := scope.Get(tgt.Name)
		return v, nil
	case *model.PropAccess:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return nil, err
		}
		obj, ok := base.(*model.Object)
		if !ok {
			return nil, fmt.Errorf("assign: %q is not an object property", tgt.Name)
		}
		return obj.Props[tgt.Name], nil
	case *model.StaticProp:
		bag, err := rt.staticStorage(tgt.Class, scope)
		if err != nil {
			return nil, err
		}
		return bag[tgt.Name], nil
	case *model.Index:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return nil, err
		}
		arr, ok := base.(*model.Array)
		if !ok {
			return nil, fmt.Errorf("assign: target is not an array")
		}
		if tgt.Index == nil {
			return nil, fmt.Errorf("assign: [] append target is write-only")
		}
		key, err := rt.Eval(tgt.Index, scope)
		if err != nil {
			return nil, err
		}
		v, _ := arr.Get(normalizeKey(key))
		return v, nil
	default:
		return nil, fmt.Errorf("assign: unsupported target %T", target)
	}
}

// assignTo writes an already-evaluated value into an lvalue (used by list()
// destructuring). Only plain `=` semantics are needed here.
func (rt *Runtime) assignTo(target model.Expr, val any, scope *Scope) error {
	switch tgt := model.UnwrapParenthesized(target).(type) {
	case *model.Var:
		rt.setVar(scope, tgt.Name, val)
		return nil
	case *model.PropAccess:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return err
		}
		if obj, ok := base.(*model.Object); ok {
			obj.SetProp(tgt.Name, val)
			return nil
		}
		return assignGoField(base, tgt.Name, func(any) (any, error) { return val, nil })
	case *model.StaticProp:
		bag, err := rt.staticStorage(tgt.Class, scope)
		if err != nil {
			return err
		}
		bag[tgt.Name] = val
		return nil
	case *model.Index:
		base, err := rt.container(tgt.Base, scope)
		if err != nil {
			return err
		}
		arr, ok := base.(*model.Array)
		if !ok {
			if tgt.Index == nil {
				return fmt.Errorf("assign: cannot append to %T; a binding whose result is appended to must return *model.Array", base)
			}
			key, err := rt.Eval(tgt.Index, scope)
			if err != nil {
				return err
			}
			return assignGoIndex(base, key, func(any) (any, error) { return val, nil })
		}
		if tgt.Index == nil {
			arr.Append(val)
			return nil
		}
		key, err := rt.Eval(tgt.Index, scope)
		if err != nil {
			return err
		}
		arr.Set(normalizeKey(key), val)
		return nil
	default:
		return fmt.Errorf("assign: unsupported list() element %T", target)
	}
}

// assignGoIndex writes into a native Go collection returned by a binding. Go
// maps accept new and replacement keys (they are reference types, so the script
// observes the write), and existing slice elements are addressable through the
// shared backing array. A slice cannot grow through the interface value holding
// it, so callers reject `$a[] =` before reaching here.
func assignGoIndex(base, key any, value func(current any) (any, error)) error {
	rv := reflect.ValueOf(base)
	switch rv.Kind() {
	case reflect.Map:
		if rv.IsNil() {
			return fmt.Errorf("assign: cannot write to a nil %T", base)
		}
		mapKey, ok := coerceArg(normalizeKey(key), rv.Type().Key())
		if !ok || !mapKey.Type().AssignableTo(rv.Type().Key()) {
			return fmt.Errorf("assign: cannot use %T as a key of %T", key, base)
		}
		var current any
		if existing := rv.MapIndex(mapKey); existing.IsValid() {
			current = existing.Interface()
		}
		updated, err := value(current)
		if err != nil {
			return err
		}
		next, ok := coerceArg(updated, rv.Type().Elem())
		if !ok || !next.Type().AssignableTo(rv.Type().Elem()) {
			return fmt.Errorf("assign: cannot assign to an element of %T", base)
		}
		rv.SetMapIndex(mapKey, next)
		return nil
	case reflect.Slice, reflect.Array:
		index := toInt(key)
		if index < 0 || index >= int64(rv.Len()) {
			return fmt.Errorf("assign: index %d is out of range for %T", index, base)
		}
		element := rv.Index(int(index))
		updated, err := value(element.Interface())
		if err != nil {
			return err
		}
		next, ok := coerceArg(updated, element.Type())
		if !element.CanSet() || !ok || !next.Type().AssignableTo(element.Type()) {
			return fmt.Errorf("assign: cannot assign to an element of %T", base)
		}
		element.Set(next)
		return nil
	}
	return fmt.Errorf("assign: target is not an array")
}

func assignGoField(base any, name string, value func(any) (any, error)) error {
	rv := reflect.ValueOf(base)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return fmt.Errorf("assign: %q is not an object property", name)
	}
	object := reflect.Indirect(rv)
	if object.Kind() != reflect.Struct {
		return fmt.Errorf("assign: %q is not an object property", name)
	}
	field := fieldByNameFold(object, name)
	if !field.IsValid() || !field.CanSet() || !field.CanInterface() {
		return fmt.Errorf("assign: %q is not a writable object property", name)
	}
	raw, err := value(field.Interface())
	if err != nil {
		return err
	}
	next, ok := coerceArg(raw, field.Type())
	if !ok || !next.Type().AssignableTo(field.Type()) {
		return fmt.Errorf("assign: cannot assign %T to property %q", raw, name)
	}
	field.Set(next)
	return nil
}

// applyAssignOp resolves compound-assignment operators against the current
// value. It reports an error for the same inputs the binary operator does, so
// `$x <<= -1` fails where `$x = $x << -1` fails instead of assigning silently.
func applyAssignOp(op string, cur, rhs any) (any, error) {
	switch op {
	case "", "=", "[]=":
		return rhs, nil
	case ".=":
		return phpString(cur) + phpString(rhs), nil
	case "+=", "-=", "*=", "/=", "%=", "**=":
		return phpArith(strings.TrimSuffix(op, "="), cur, rhs), nil
	case "&=", "|=", "^=", "<<=", ">>=":
		return phpBitwise(strings.TrimSuffix(op, "="), cur, rhs)
	default:
		return rhs, nil
	}
}

// invokeFunc runs a user-defined function in a fresh scope.
func (rt *Runtime) invokeFunc(decl *model.FuncDecl, args []any) (any, error) {
	scope := rt.newScope()
	rt.pushFrame(scope)
	defer rt.popFrame()
	if decl.Filename != "" {
		setScopeFile(scope, decl.Filename)
	}
	scope.Set(argsKey, args)
	if err := rt.bindParams(decl, args, scope); err != nil {
		return nil, err
	}
	val, _, runErr := rt.exec(decl.Body, scope)
	return val, combineErrors(runErr, rt.runDeferred(scope, 0))
}

// invokeMethod runs a method with $this bound to obj. The invocation span uses
// the caller scope; the fresh scope below identifies where the method body is
// defined and is used for spans created from within that body.
func (rt *Runtime) invokeMethod(obj *model.Object, decl *model.FuncDecl, args []any, caller *Scope) (any, error) {
	scope := rt.newScope()
	rt.pushFrame(scope)
	defer rt.popFrame()
	if decl.Filename != "" {
		setScopeFile(scope, decl.Filename)
	}
	scope.Set("this", obj)
	scope.Set(argsKey, args)
	if obj.Class != nil {
		scope.Set("__class__", obj.Class.Name)
	}
	// The span name costs two string builds, so it is only assembled when an
	// observer is listening.
	if len(rt.observers) > 0 {
		name := obj.ID
		if name == "" && obj.Class != nil {
			name = obj.Class.Name
		}
		if name != "" {
			name += "."
		}
		traceScope := caller
		if traceScope == nil {
			traceScope = scope
		}
		defer rt.trace(traceScope, name+decl.Name)()
	}
	if err := rt.bindParams(decl, args, scope); err != nil {
		return nil, err
	}
	val, _, runErr := rt.exec(decl.Body, scope)
	return val, combineErrors(runErr, rt.runDeferred(scope, 0))
}

// closureEnv is everything a closure carries away from the scope it was written
// in: the lexical magic constants, the `use (...)` captures, and the bound
// `$this` (nil for a `static function`, or for one written outside a method).
// It is built once where the closure value is created, not on each call, which
// is what makes the capture a snapshot the way PHP's by-value `use` is.
type closureEnv struct {
	filename  any
	directory any
	captured  map[string]any
	this      any
	class     any

	// statics holds this closure value's function-static bags: PHP gives every
	// closure instance its own `static $x` storage, so two counters built by
	// the same factory count independently. Keyed like Runtime.funcStatics.
	statics map[*model.StaticVar]map[string]any
}

// captureClosureEnv snapshots scope for a closure declared in it.
func captureClosureEnv(cl *model.Closure, scope *Scope) closureEnv {
	env := closureEnv{statics: map[*model.StaticVar]map[string]any{}}
	env.filename, _ = scope.Get("__FILE__")
	env.directory, _ = scope.Get("__DIR__")
	if !cl.Static {
		env.this, _ = scope.Get("this")
		env.class, _ = scope.Get("__class__")
	}
	if len(cl.Uses) > 0 {
		env.captured = make(map[string]any, len(cl.Uses))
		for _, use := range cl.Uses {
			// A by-reference capture binds the same value a by-value one does:
			// the runtime has no reference cells, so the distinction only shows
			// up in writes the closure makes back to the enclosing frame, which
			// it cannot do either way.
			value, _ := scope.Get(use.Name)
			env.captured[use.Name] = value
		}
	}
	return env
}

// invokeClosure runs an anonymous function in a fresh scope seeded with its
// captured environment. Parameters are bound after the captures, so a parameter
// of the same name shadows the capture, as it does in PHP.
func (rt *Runtime) invokeClosure(cl *model.Closure, args []any, env closureEnv) (any, error) {
	scope := rt.newScope()
	rt.pushFrame(scope)
	defer rt.popFrame()
	scope.Set(argsKey, args)
	if env.filename != nil {
		scope.Set("__FILE__", env.filename)
	}
	if env.directory != nil {
		scope.Set("__DIR__", env.directory)
	}
	if env.this != nil {
		scope.Set("this", env.this)
	}
	if env.class != nil {
		scope.Set("__class__", env.class)
	}
	for name, value := range env.captured {
		scope.Set(name, value)
	}
	scope.Set(staticsKey, env.statics)
	decl := &model.FuncDecl{Params: cl.Params, Body: cl.Body}
	if err := rt.bindParams(decl, args, scope); err != nil {
		return nil, err
	}
	val, _, runErr := rt.exec(cl.Body, scope)
	return val, combineErrors(runErr, rt.runDeferred(scope, 0))
}

// runDeferred invokes callbacks registered since mark in last-in, first-out
// order. Each callback is removed before invocation so it runs at most once,
// including when it fails or registers another callback during the unwind.
func (rt *Runtime) runDeferred(scope *Scope, mark int) error {
	var errs []error
	for len(scope.deferred) > mark {
		i := len(scope.deferred) - 1
		callback := scope.deferred[i]
		scope.deferred[i] = nil
		scope.deferred = scope.deferred[:i]
		if _, err := rt.invokeWithScopeContext(callback, nil, scope); err != nil {
			errs = append(errs, err)
		}
	}
	return combineErrors(errs...)
}

func (rt *Runtime) runShutdown() error {
	if len(rt.shutdown) == 0 {
		return nil
	}
	var errs []error
	scope := rt.newScope()
	rt.pushFrame(scope)
	defer rt.popFrame()
	for len(rt.shutdown) > 0 {
		callback := rt.shutdown[0]
		rt.shutdown[0] = nil
		rt.shutdown = rt.shutdown[1:]
		if _, err := rt.invokeWithScopeContext(callback, nil, scope); err != nil {
			errs = append(errs, err)
		}
	}
	return combineErrors(errs...)
}

// combineErrors preserves an existing error unchanged when there is only one,
// while retaining every error when execution and one or more defers fail.
func combineErrors(errs ...error) error {
	nonNil := errs[:0]
	for _, err := range errs {
		if err != nil {
			nonNil = append(nonNil, err)
		}
	}
	switch len(nonNil) {
	case 0:
		return nil
	case 1:
		return nonNil[0]
	default:
		return errors.Join(nonNil...)
	}
}

// argsKey is the scope slot holding the current call's positional arguments so
// func_get_args() can return them.
const argsKey = "__args__"

// bindParams binds positional args to parameter names, applying defaults. A
// variadic parameter collects every remaining argument into one array, an
// empty one when the caller stopped short of it, and is the last binding
// either way, as the parser allows `...` only in final position.
func (rt *Runtime) bindParams(decl *model.FuncDecl, args []any, scope *Scope) error {
	for i, p := range decl.Params {
		if p.Variadic {
			n := 0
			if len(args) > i {
				n = len(args) - i
			}
			rest := model.NewArraySize(n)
			for _, arg := range args[i:] {
				rest.Append(arg)
			}
			scope.Set(p.Name, rest)
			return nil
		}
		if i < len(args) {
			scope.Set(p.Name, args[i])
			continue
		}
		if p.Default != nil {
			v, err := rt.Eval(p.Default, scope)
			if err != nil {
				return err
			}
			scope.Set(p.Name, v)
			continue
		}
		scope.Set(p.Name, nil)
	}
	return nil
}
