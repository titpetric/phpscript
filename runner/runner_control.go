package runner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// This file executes the compound statements: the loops, try/catch/finally
// with the clause matching both engines share, and switch. runner.go holds the
// statement dispatcher that reaches them.

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
	// PHP's two loop semantics. `as &$v` binds the element; a body that
	// assigns to the target edits the source. `as $v` binds a copy, which
	// edits nothing. Only a *model.Array accepts a write-back or a copy. A
	// collection a binding returned belongs to the host.
	//
	// The copy happens only when the body actually assigns through the target,
	// because phpscript has no refcount to defer it with: an unconditional copy
	// would charge every loop for a semantic almost none of them use. The test
	// runs once per loop, not once per iteration.
	var writable *model.Array
	if n.ByRef {
		writable, _ = src.(*model.Array)
	}
	copyValue := !n.ByRef && modelutil.AssignsTo(valTarget, n.Body)

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
			v = modelutil.CopyValue(v)
		}
		if err = rt.assignTo(valTarget, v, scope); err != nil {
			return false
		}
		var fl flow
		val, fl, err = rt.exec(n.Body, scope)
		if err != nil {
			return false
		}
		// The write-back happens before the switch reads the flow. A reference
		// makes the element live. A body that assigns, then breaks or returns,
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
	case model.Collection:
		src.Range(iter)
	case *model.Object:
		// An object visits its properties, name and value, in the order it
		// reads them back. PHP visits only the ones visible where the script
		// writes the loop; phpscript enforces no visibility anywhere and it
		// visits them all. See docs/README.md.
		src.Range(func(name string, value any) bool {
			return iter(name, value)
		})
	default:
		// Native Go collections (e.g. a []Record returned by a forwarded
		// method) are iterable too: slices/arrays by integer index, maps by
		// key. The walk is model's, where a []byte reads as the string it
		// holds. A []byte iterates zero times, as a string does.
		//
		// A source no shape above matched iterates zero times too, as PHP's
		// foreach across a non-array does: it warns and continues.
		modelutil.RangeValues(src, iter)
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

// execDoWhile runs the body once, continue falling through to the check.
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

// execTry runs the body, matching catch or finally, and passes exit() through uncatchable.
func (rt *Runtime) execTry(n *model.Try, scope *Scope) (any, flow, error) {
	val, fl, err := rt.exec(n.Body, scope)
	if _, exiting := IsExit(err); exiting {
		// execTry skips finally too: PHP runs no finally block on exit.
		return val, fl, err
	}
	// Running out of time, or losing the client, is not a condition the script
	// gets to handle and resume from: the clock has run out, and the next
	// statement would raise it again. Cleanup goes in a
	// register_shutdown_function callback, which runs with the clock off.
	var limit *TimeLimitError
	if errors.As(err, &limit) {
		return val, fl, err
	}

	if err != nil && len(n.Catches) > 0 {
		caught := rt.catchError(err)

		for _, c := range n.Catches {
			if rt.matchCatch(c.Type, caught) {
				if c.Var != "" {
					scope.Set(c.Var, catchValue(caught))
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

// matchCatchType reports whether a catch type handles rootErr, a union any alternative.
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

// catchError answers the error a clause sees: the root cause under php, the thrown error under go.
func (rt *Runtime) catchError(err error) error {
	if rt.GoRuntime() {
		return err
	}
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err
		}
		err = unwrapped
	}
}

// matchCatch answers the clause by the runtime's rules: go catches error.
func (rt *Runtime) matchCatch(declaredType string, rootErr error) bool {
	if !rt.GoRuntime() {
		return matchCatchType(declaredType, rootErr)
	}
	want := strings.TrimPrefix(strings.TrimSpace(declaredType), "\\")
	return want == "" || strings.EqualFold(want, "error")
}

// throwValue turns a thrown value into the error it travels as.
func (rt *Runtime) throwValue(value any) error {
	if thrown, ok := value.(error); ok {
		return thrown
	}
	if rt.GoRuntime() {
		return &TypeError{Name: "throw", Position: 1, Want: "error", Got: phpDebugType(value)}
	}
	if obj, ok := value.(*model.Object); ok {
		return newObjectError(obj)
	}
	return fmt.Errorf("uncaught exception: %s", phpString(value))
}

// execSwitch runs matching case bodies with PHP fall-through until break or return.
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
