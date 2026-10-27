package runner

import (
	"errors"
	"reflect"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// This file executes the compound statements: the loops, try/catch/finally
// with the clause matching both engines share, and switch. The statement
// dispatcher that reaches them lives in runner.go.

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
	// PHP's two loop semantics. `as &$v` binds the element and a body that
	// assigns to the target edits the source; `as $v` binds a copy and it does
	// not. Only a *model.Array accepts a write-back or a copy; a collection
	// a binding returned belongs to the host and not to the script.
	//
	// The copy happens only when the body actually assigns through the target,
	// because phpscript has no refcount to defer it with: an unconditional copy
	// would charge every loop for a semantic almost none of them use. The test
	// runs once per loop, not once per iteration.
	var writable *model.Array
	if n.ByRef {
		writable, _ = src.(*model.Array)
	}
	copyValue := !n.ByRef && model.AssignsTo(valTarget, n.Body)

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
		// The write-back happens before the switch reads the flow: a reference
		// makes the element live and a body that assigns and then breaks or
		// returns has still edited the source.
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
		// execForeach skips any other non-iterable source (PHP warns and
		// continues).
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

// execDoWhile runs the body before the first condition check and it executes
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
// exit() and die() are not errors and are not catchable, as in PHP: `try {
// exit(); } catch (Throwable $e) {}` ends the script there. They
// travel as an error here only because that is how the interpreter unwinds and
// the try has to recognise the sentinel and get out of the way. A catch that
// could swallow an exit would turn `header("Location: ...") ; exit();` inside a
// try into a page that carries on running past a staged redirect.
func (rt *Runtime) execTry(n *model.Try, scope *Scope) (any, flow, error) {
	val, fl, err := rt.exec(n.Body, scope)
	if _, exiting := IsExit(err); exiting {
		// execTry skips finally too: PHP runs no finally block on exit.
		return val, fl, err
	}
	// Running out of time, or losing the client, is not a condition the script
	// gets to handle and carry on from: there is no more time to carry on with,
	// and the next statement would raise it again. Cleanup goes in a
	// register_shutdown_function callback, which runs with the clock off.
	var limit *TimeLimitError
	if errors.As(err, &limit) {
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
// There is no class hierarchy to walk and matchCatchType answers the clause
// from the name the throwable records:
//
//   - nothing, or Throwable, takes everything.
//   - an error that is no PHP class at all takes any clause and a Go binding's
//     error reaches the catch a script already wrote around the call.
//   - Exception takes any class whose name does not end in "Error", and Error
//     takes any class whose name does. That suffix is the whole of the split
//     between a fault in the program and a condition it raised, and it agrees
//     with PHP for every built-in name: ErrorException is an Exception,
//     AssertionError and TypeError are Errors.
//   - any other name matches the class case-insensitively.
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
