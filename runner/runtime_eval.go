package runner

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner/expr"
)

// This file is Runtime.Eval and its machinery: compiling an expression to the
// closure engine, the pooled evaluation environments the compiled chains run
// in, variable resolution, and the per-runtime helpers the environments
// install. The engine itself lives in runner/expr.

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

// ScopeBuiltins names the builtins the function table does not carry.
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
