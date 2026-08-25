package runner

import "github.com/titpetric/phpscript/model"

// objectError carries an instance of a declared class that a script threw.
//
// An object is not an error, so throwing one needs a wrapper to travel through
// the same path a built-in throwable takes. The wrapper records the one class
// the object was declared as, which is what a clause filters on and what
// get_class() reports. It records nothing else: a class declaring `extends` is
// still only its own class here, because there is no inheritance.
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

// ThrowableClass returns the class the object was declared as.
func (e *objectError) ThrowableClass() string {
	if e.object == nil || e.object.Class == nil {
		return ""
	}
	return e.object.Class.Name
}

// Value returns the object a catch clause binds, so a script gets back what it
// threw rather than the wrapper.
func (e *objectError) Value() any { return e.object }

// catchValue returns what a catch clause binds for err: the object for a thrown
// instance, and the error itself for everything else.
func catchValue(err error) any {
	if oe, ok := err.(*objectError); ok {
		return oe.Value()
	}
	return err
}
