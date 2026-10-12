package expr

import (
	"github.com/titpetric/phpscript/model"
)

// BoundClosure is one compiled closure literal: the synthetic identifier,
// the declaration Eval binds a callable for, and its slot.
type BoundClosure struct {
	ID   string
	Slot int
	Decl *model.Closure
}
