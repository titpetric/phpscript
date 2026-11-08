package runner

import (
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// coalesceAssign stores value where target reads null, as `$target ??= value`,
// and answers what the target holds afterwards. It resolves the target once:
// `$a[k()] ??= 1` calls k once, as php does.
func (rt *Runtime) coalesceAssign(target, value model.Expr, scope *Scope) (any, error) {
	switch tgt := modelutil.UnwrapParenthesized(target).(type) {
	case *model.Index:
		if tgt.Index != nil {
			return rt.coalesceIndex(tgt, value, scope)
		}
	case *model.PropAccess:
		return rt.coalesceProp(tgt, value, scope)
	}
	// A variable and a static property name their storage and run no
	// expression to reach it: the generic path reads the name twice.
	cur, err := rt.Eval(target, scope)
	if err != nil || cur != nil {
		return cur, err
	}
	rhs, err := rt.Eval(value, scope)
	if err != nil {
		return nil, err
	}
	return rhs, rt.assignTo(target, rhs, scope)
}

func (rt *Runtime) coalesceIndex(tgt *model.Index, value model.Expr, scope *Scope) (any, error) {
	base, err := rt.container(tgt.Base, scope)
	if err != nil {
		return nil, err
	}
	key, err := rt.Eval(tgt.Index, scope)
	if err != nil {
		return nil, err
	}
	if cur := helperIndex(base, key); cur != nil {
		return cur, nil
	}
	rhs, err := rt.Eval(value, scope)
	if err != nil {
		return nil, err
	}
	return rhs, storeIndex(base, key, rhs)
}

func (rt *Runtime) coalesceProp(tgt *model.PropAccess, value model.Expr, scope *Scope) (any, error) {
	base, err := rt.Eval(tgt.Base, scope)
	if err != nil {
		return nil, err
	}
	name, err := rt.propName(tgt, scope)
	if err != nil {
		return nil, err
	}
	if cur := rt.helperGet(&scopeRef{scope: scope})(base, name); cur != nil {
		return cur, nil
	}
	rhs, err := rt.Eval(value, scope)
	if err != nil {
		return nil, err
	}
	return rhs, storeProp(base, name, rhs)
}
