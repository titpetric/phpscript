package runner

import (
	"github.com/titpetric/phpscript/model"
)

// objectError holds a thrown declared-class instance and records its one class.
type objectError struct {
	object *model.Object
}

// newObjectError wraps a thrown object.
func newObjectError(obj *model.Object) *objectError {
	return &objectError{object: obj}
}

// Error renders the object the way an uncaught throw reports it.
func (e *objectError) Error() string {
	if e.object == nil {
		return "uncaught exception"
	}
	name := "object"
	if e.object.Class != nil {
		name = e.object.Class.Name
	}
	if message, ok := e.object.Props["message"].(string); ok && message != "" {
		return name + ": " + message
	}
	return "uncaught exception: " + name
}

// ThrowableClass returns the class the object declares.
func (e *objectError) ThrowableClass() string {
	if e.object == nil || e.object.Class == nil {
		return ""
	}
	return e.object.Class.Name
}

// Value returns the thrown object a catch clause binds, never the wrapper.
func (e *objectError) Value() any { return e.object }

// catchValue returns a thrown instance's object, and err itself for everything else.
func catchValue(err error) any {
	if oe, ok := err.(*objectError); ok {
		return oe.Value()
	}
	return err
}
