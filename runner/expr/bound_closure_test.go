package expr

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestBoundClosure covers what Eval binds a callable from. The declaration is
// AST and the identifier is synthetic: a closure literal has no name a script
// typed, so the compiler invents one and the slot is where the callable lands.
func TestBoundClosure(t *testing.T) {
	t.Parallel()
	decl := &model.Closure{Params: []model.Param{{Name: "n"}}}
	bound := BoundClosure{ID: "__closure_0", Slot: 1, Decl: decl}

	if bound.Decl != decl {
		t.Error("BoundClosure lost the declaration it was given")
	}
	if bound.ID == "" {
		t.Error("BoundClosure carries no identifier, so Eval has nothing to key the callable on")
	}
	if len(bound.Decl.Params) != 1 {
		t.Errorf("the declaration declares %d parameters, want 1", len(bound.Decl.Params))
	}

	if zero := (BoundClosure{}); zero.Decl != nil || zero.ID != "" {
		t.Errorf("the zero BoundClosure is not empty: %+v", zero)
	}
}
