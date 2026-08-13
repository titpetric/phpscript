package runner

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
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

// Load parses PHP source into a program.
func (rt *Runtime) Load(src string) (*model.Program, error) {
	rt.UpdateStatus(model.StatusReading)
	program, err := parser.Parse(src)
	if err != nil {
		rt.UpdateStatus(model.StatusError)
	}
	return program, err
}

// LoadFile reads and parses a PHP file from the runtime source FS.
func (rt *Runtime) LoadFile(path string) (*model.Program, error) {
	rt.UpdateFilename(path)
	rt.UpdateStatus(model.StatusReading)
	if rt.opts.RootFS == nil {
		rt.UpdateStatus(model.StatusError)
		return nil, fmt.Errorf("load %q: no source FS configured", path)
	}
	cleanPath := rt.resolveFSPath(path)
	b, err := fs.ReadFile(rt.opts.RootFS, cleanPath)
	if err != nil {
		rt.UpdateStatus(model.StatusError)
		return nil, fmt.Errorf("load %q: %w", path, err)
	}
	prog, err := rt.Load(string(b))
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
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
		rt.UpdateStatus(model.StatusError)
		if _, exit := IsExit(err); exit {
			return
		}
		ctx := rt.ctx
		if rt.entrypoint != "" {
			ctx = model.WithSpanFilename(ctx, rt.entrypoint)
		}
		span := rt.traceContext(ctx, fmt.Sprintf("Error: <code>%s</code>", template.HTMLEscapeString(err.Error())))
		if span != nil {
			span.RecordError(err)
		}
	}()
	rt.UpdateStatus(model.StatusProcessing)
	if rt.flat {
		if handled, flatErr := rt.runFlat(p); handled {
			err = flatErr
			return err
		}
	}
	err = rt.runInterpreted(p)
	return err
}

func (rt *Runtime) runInterpreted(p *model.Program) error {
	scope := NewScope()
	for name, val := range rt.globals {
		scope.Set(name, val)
	}
	if rt.entrypoint != "" {
		scope.Set("__FILE__", rt.entrypoint)
		scope.Set("__DIR__", path.Dir(rt.entrypoint))
	}
	// Hoist declarations so functions/classes are callable before their textual
	// position (PHP semantics for top-level function/class definitions).
	if err := rt.hoist(p.Stmts, rt.entrypoint); err != nil {
		return err
	}
	_, _, runErr := rt.exec(p.Stmts, scope)
	return combineErrors(runErr, rt.runDeferred(scope, 0))
}

// hoist registers all function and class declarations found at the given level.
func (rt *Runtime) hoist(stmts []model.Stmt, filename string) error {
	// First pass: classes, so methods can be attached.
	classes := map[string]*model.Class{}
	for _, s := range stmts {
		if cd, ok := s.(*model.ClassDecl); ok {
			c := &model.Class{Name: cd.Name, Fields: cd.Fields, Consts: cd.Consts, Methods: map[string]*model.FuncDecl{}}
			for _, m := range cd.Methods {
				m.Filename = filename
				c.Methods[m.Name] = m
			}
			classes[cd.Name] = c
			rt.RegisterClass(c)
		}
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
		rt.UpdateStatus(model.StatusProcessing)
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
		rt.UpdateStatus(model.StatusWriting)
		_, err := io.WriteString(rt.out, n.Text)
		return nil, flowNormal, err

	case *model.Echo:
		for _, a := range n.Args {
			v, err := rt.Eval(a, scope)
			if err != nil {
				return nil, flowNormal, err
			}
			rt.UpdateStatus(model.StatusWriting)
			if _, err := io.WriteString(rt.out, phpString(v)); err != nil {
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

	case *model.Break:
		return nil, flowBreak, nil

	case *model.Continue:
		return nil, flowContinue, nil

	case *model.Throw:
		v, err := rt.Eval(n.X, scope)
		if err != nil {
			return nil, flowNormal, err
		}
		return nil, flowNormal, fmt.Errorf("uncaught exception: %s", phpString(v))

	case *model.FuncDecl, *model.ClassDecl:
		// Already handled by hoist.
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
	// iter runs the loop body for one (key, value) pair, returning whether to
	// continue iterating. It records any error / non-normal flow in the
	// enclosing variables.
	iter := func(k, v any) bool {
		if keyTarget != nil {
			if err = rt.assignTo(keyTarget, k, scope); err != nil {
				return false
			}
		}
		if err = rt.assignTo(valTarget, v, scope); err != nil {
			return false
		}
		var fl flow
		val, fl, err = rt.exec(n.Body, scope)
		if err != nil {
			return false
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

// execTry runs the try body; if it raises an error (a throw or a runtime error
// from a forwarded Go call), the first catch clause handles it with the error
// bound to its variable (so `echo $e` prints the message). A finally block, if
// present, always runs. There is no exception class hierarchy, so the first
// catch catches everything.
func (rt *Runtime) execTry(n *model.Try, scope *Scope) (any, flow, error) {
	val, fl, err := rt.exec(n.Body, scope)
	if err != nil && len(n.Catches) > 0 {
		c := n.Catches[0]
		if c.Var != "" {
			// Bind the root cause so PHP sees the original message, not the
			// transpiler's "eval ..."/"compile ..." wrapping.
			for {
				if unwrapped := errors.Unwrap(err); unwrapped != nil {
					err = unwrapped
					continue
				}
				break
			}
			scope.Set(c.Var, err)
		}
		val, fl, err = rt.exec(c.Body, scope)
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
	return rt.includeFile(phpString(path), scope)
}

func (rt *Runtime) trace(scope *Scope, message string) func() {
	span := rt.traceContext(contextWithScope(rt.ctx, scope), message)
	return func() {
		if span != nil {
			span.End()
		}
	}
}

func (rt *Runtime) traceRegion(scope *Scope, message string, flags ...model.Flag) func() {
	ctx := contextWithScope(rt.ctx, scope)
	rt.traceContext(ctx, message, append(flags, model.OpenSpan)...)
	return func() {
		rt.traceContext(ctx, message, append(flags, model.CloseSpan)...)
	}
}

func (rt *Runtime) includeFile(path string, scope *Scope) (any, error) {
	var spanType model.Flag
	if strings.EqualFold(filepath.Ext(path), ".tpl") {
		spanType = model.SpanType.Template
	}
	defer rt.traceRegion(scope, "include "+path, spanType)()

	prog, filename, err := rt.resolveInclude(path)
	if err != nil {
		return nil, fmt.Errorf("error including %s: %w", path, err)
	}
	rt.included = append(rt.included, filename)
	rt.UpdateIncludedFiles(len(rt.included))
	restoreFile := setScopeFile(scope, filename)
	defer restoreFile()
	if err := rt.hoist(prog.Stmts, filename); err != nil {
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
		callable := loader
		if name, ok := loader.(string); ok {
			var found bool
			callable, found = rt.lookupFunc(name)
			if !found {
				return fmt.Errorf("autoload callback %q is not callable", name)
			}
		}
		if _, err := rt.invokeWithScopeContext(callable, []any{class}, scope); err != nil {
			return err
		}
		if rt.hasClass(class) {
			return nil
		}
	}
	return nil
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
			_, err := rt.includeFile(filename, NewScope())
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
		cleanPath := cleanFSPath(path)
		if prog, ok := rt.includeCache.Get(cleanPath); ok {
			return prog, cleanPath, nil
		}

		prog, err := rt.LoadFile(cleanPath)
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

func (rt *Runtime) resolveFSPath(p string) string {
	cleanPath := cleanFSPath(p)
	if rt.opts.WorkDir == "" || rt.opts.WorkDir == "." || cleanPath == "." {
		return cleanPath
	}
	return cleanFSPath(path.Join(rt.opts.WorkDir, cleanPath))
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
		scope.Set(tgt.Name, applyAssignOp(n.Op, cur, rhs))
		return nil

	case *model.ListExpr:
		// list($a, $b) = $arr — destructure by position (integer keys).
		arr, ok := rhs.(*model.Array)
		if !ok {
			return fmt.Errorf("assign: list() target requires an array")
		}
		for i, el := range tgt.Elems {
			if el == nil {
				continue
			}
			v, _ := arr.Get(int64(i))
			if err := rt.assignTo(el, v, scope); err != nil {
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
			obj.Props[tgt.Name] = applyAssignOp(n.Op, obj.Props[tgt.Name], rhs)
			return nil
		}
		return assignGoField(base, tgt.Name, func(current any) any {
			return applyAssignOp(n.Op, current, rhs)
		})

	case *model.Index:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return err
		}
		arr, ok := base.(*model.Array)
		if !ok {
			return fmt.Errorf("assign: target is not an array")
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
		arr.Set(k, applyAssignOp(n.Op, cur, rhs))
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
		scope.Set(tgt.Name, val)
		return nil
	case *model.PropAccess:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return err
		}
		if obj, ok := base.(*model.Object); ok {
			obj.Props[tgt.Name] = val
			return nil
		}
		return assignGoField(base, tgt.Name, func(any) any { return val })
	case *model.Index:
		base, err := rt.Eval(tgt.Base, scope)
		if err != nil {
			return err
		}
		arr, ok := base.(*model.Array)
		if !ok {
			return fmt.Errorf("assign: target is not an array")
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

func assignGoField(base any, name string, value func(any) any) error {
	rv := reflect.ValueOf(base)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return fmt.Errorf("assign: %q is not an object property", name)
	}
	object := reflect.Indirect(rv)
	if object.Kind() != reflect.Struct {
		return fmt.Errorf("assign: %q is not an object property", name)
	}
	field := object.FieldByName(name)
	if !field.IsValid() {
		field = object.FieldByNameFunc(func(fieldName string) bool {
			return strings.EqualFold(fieldName, name)
		})
	}
	if !field.IsValid() || !field.CanSet() || !field.CanInterface() {
		return fmt.Errorf("assign: %q is not a writable object property", name)
	}
	raw := value(field.Interface())
	next := coerceArg(raw, field.Type())
	if !next.IsValid() || !next.Type().AssignableTo(field.Type()) {
		return fmt.Errorf("assign: cannot assign %T to property %q", raw, name)
	}
	field.Set(next)
	return nil
}

// applyAssignOp resolves compound-assignment operators against the current value.
func applyAssignOp(op string, cur, rhs any) any {
	switch op {
	case "", "=", "[]=":
		return rhs
	case ".=":
		return phpString(cur) + phpString(rhs)
	case "+=":
		return toInt(cur) + toInt(rhs)
	case "-=":
		return toInt(cur) - toInt(rhs)
	default:
		return rhs
	}
}

// invokeFunc runs a user-defined function in a fresh scope.
func (rt *Runtime) invokeFunc(decl *model.FuncDecl, args []any) (any, error) {
	scope := NewScope()
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

// invokeMethod runs a method with $this bound to obj.
func (rt *Runtime) invokeMethod(obj *model.Object, decl *model.FuncDecl, args []any) (any, error) {
	scope := NewScope()
	if decl.Filename != "" {
		setScopeFile(scope, decl.Filename)
	}
	scope.Set("this", obj)
	scope.Set(argsKey, args)
	if obj.Class != nil {
		scope.Set("__class__", obj.Class.Name)
	}
	name := obj.ID
	if name == "" && obj.Class != nil {
		name = obj.Class.Name
	}
	if name != "" {
		name += "."
	}
	defer rt.trace(scope, name+decl.Name)()
	if err := rt.bindParams(decl, args, scope); err != nil {
		return nil, err
	}
	val, _, runErr := rt.exec(decl.Body, scope)
	return val, combineErrors(runErr, rt.runDeferred(scope, 0))
}

// invokeClosure runs an anonymous function in a fresh scope. User variables are
// not captured, but lexical magic constants retain their defining filename.
func (rt *Runtime) invokeClosure(cl *model.Closure, args []any, filename, directory any) (any, error) {
	scope := NewScope()
	scope.Set(argsKey, args)
	if filename != nil {
		scope.Set("__FILE__", filename)
	}
	if directory != nil {
		scope.Set("__DIR__", directory)
	}
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
	var errs []error
	scope := NewScope()
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

// bindParams binds positional args to parameter names, applying defaults.
func (rt *Runtime) bindParams(decl *model.FuncDecl, args []any, scope *Scope) error {
	for i, p := range decl.Params {
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
