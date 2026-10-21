package expr

import (
	"fmt"
	"strconv"

	"github.com/titpetric/phpscript/model"
)

// Compiled is a model.Expr compiled straight to a closure chain, with the
// bindings Eval fills before running: variables by slot, closure literals by
// slot, and the registered-function names to install into the base env. It
// carries no bytecode; the string round-trip through the transpiler and
// expr-lang's parser never happens for an expression compiled here.
type Compiled struct {
	Program  *Program
	Vars     []BoundVar
	Calls    []string
	Closures []BoundClosure
	// Exprs holds the marked sub-expressions (`++`/`--`, include) the
	// compiled closures run through the __eval base helper, keyed by
	// the mark id, the same contract the transpiler's marks use.
	Exprs map[string]model.Expr
}

// CompileExpr compiles a model.Expr directly to a closure chain. It covers
// the whole expression vocabulary the runtime evaluates; a shape outside
// that vocabulary is an error, spelled the way the transpiler spells one.
func CompileExpr(e model.Expr, h *Helpers) (*Compiled, error) {
	dc := &directCompiler{h: h, slots: map[string]int{}}
	body, err := dc.compile(e)
	if err != nil {
		return nil, err
	}
	if len(dc.exprs) > 0 && len(dc.vars) > 0 {
		// A marked sub-expression writes the scope mid-evaluation, and a
		// slot snapshot taken before the run would miss it: $w++ . $w must
		// read the incremented $w. Recompile with variables read live
		// through the __var helper.
		dc = &directCompiler{h: h, liveVars: true, slots: map[string]int{}}
		if body, err = dc.compile(e); err != nil {
			return nil, err
		}
	}
	prog := &Program{slots: dc.slots}
	prog.run = wrapRoot(h, body)
	return &Compiled{
		Program:  prog,
		Vars:     dc.vars,
		Calls:    dc.calls,
		Closures: dc.closures,
		Exprs:    dc.exprs,
	}, nil
}

type directCompiler struct {
	h        *Helpers
	liveVars bool
	slots    map[string]int
	vars     []BoundVar
	calls    []string
	callSeen map[string]struct{}
	closures []BoundClosure
	exprs    map[string]model.Expr
}

// markCall registers e as a marked sub-expression and compiles to an __eval
// call on its id: the runtime evaluates the node itself, with live scope
// reads and lvalue writes the closure environment cannot express.
func (dc *directCompiler) markCall(e model.Expr) closure {
	if dc.exprs == nil {
		dc.exprs = map[string]model.Expr{}
	}
	id := "__m" + strconv.Itoa(len(dc.exprs))
	dc.exprs[id] = e
	return baseCall("__eval", []closure{constClosure(id)})
}

// bindVar returns the slot for a PHP variable or bare name, registering the
// binding on first sight. The slot key reuses the transpiler's identifier
// spelling (`v_`, `c_`, bare `this`) so one variable referenced twice shares
// one slot.
func (dc *directCompiler) bindVar(name string, isConst bool) int {
	ident := "v_" + name
	if isConst {
		ident = constIdentPrefix + name
	} else if name == "this" {
		ident = "this"
	}
	if slot, ok := dc.slots[ident]; ok {
		return slot
	}
	slot := len(dc.slots)
	dc.slots[ident] = slot
	dc.vars = append(dc.vars, BoundVar{Name: name, Const: isConst, Slot: slot})
	return slot
}

const constIdentPrefix = "c_"

func (dc *directCompiler) addCall(name string) {
	if _, ok := dc.callSeen[name]; ok {
		return
	}
	if dc.callSeen == nil {
		dc.callSeen = map[string]struct{}{}
	}
	dc.callSeen[name] = struct{}{}
	dc.calls = append(dc.calls, name)
}

func slotRead(slot int) closure {
	return func(env *Env) (any, error) {
		return env.Vars[slot], nil
	}
}

// baseCall evaluates args and calls the named base-env entry: a registered
// function installed by Eval, or one of the per-runtime helpers (__call,
// __get, __new, ...). The []any argument slice is their variadic signature.
// A name in the scratch set borrows the environment's arena for it; every
// other callee gets a fresh slice, because a user function retains its
// arguments for func_get_args and a registered binding may store its
// variadic tail.
func baseCall(name string, args []closure) closure {
	if scratchName(name) {
		return scratchCall(name, args)
	}
	return func(env *Env) (any, error) {
		fn, ok := env.Base[name].(func(...any) (any, error))
		if !ok {
			return nil, fmt.Errorf("cannot call %s: not a function", name)
		}
		in := make([]any, len(args))
		for i, c := range args {
			v, err := c(env)
			if err != nil {
				return nil, err
			}
			in[i] = v
		}
		return fn(in...)
	}
}

// scratchName lists the helper entries whose argument slice provably ends
// with the call: each reads its arguments as elements and stores none of the
// slice. __call is in the set because the one sink that retains a frame's
// arguments, the PHP-method branch of runner.helperCall, clones what it
// hands to the frame. The dispatchers that reach a user frame or a registered
// binding with the slice itself (__func, __invoke, __static, __new, __eval
// and every registered name) stay off the arena.
func scratchName(name string) bool {
	switch name {
	case "__get", "__set", "__var", "__classconst", "__staticprop",
		"__callablemember", "__callable", "__ref", "__call":
		return true
	}
	return false
}

// scratchCall is baseCall over the environment's arena: it pushes the
// arguments, borrows the region for the call, and the pop returns it, so a
// covered call boxes no slice per invocation. The three-index expression caps
// the borrowed region, so an append inside the callee reallocates away from
// the arena in place of writing over it.
func scratchCall(name string, args []closure) closure {
	return func(env *Env) (any, error) {
		fn, ok := env.Base[name].(func(...any) (any, error))
		if !ok {
			return nil, fmt.Errorf("cannot call %s: not a function", name)
		}
		base := len(env.Scratch)
		for _, c := range args {
			v, err := c(env)
			if err != nil {
				env.Scratch = env.Scratch[:base]
				return nil, err
			}
			env.Scratch = append(env.Scratch, v)
		}
		in := env.Scratch[base:len(env.Scratch):len(env.Scratch)]
		result, err := fn(in...)
		env.Scratch = env.Scratch[:base]
		return result, err
	}
}
