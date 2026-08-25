package runner

// phpscript has no OOP. There is no class hierarchy, no inheritance and no
// `extends` at runtime.
//
// A catch clause is selected by the name a throwable records, by the rule in
// matchCatchType, and `instanceof` is class-name equality. `extends` is parsed
// and recorded on model.ClassDecl so the formatter can print it back and the
// linter can see it; nothing in runner may read it.
//
// Do not add a parent table, a Parent field on model.Class, or a hierarchy
// walk. That design was proposed and rejected. See docs/design.md.

// Throwable is implemented by a value that knows which PHP class it was
// constructed as.
//
// The class is asked for rather than read off the Go type, because every SPL
// name is one Go type: reflection would answer "Exception" for an
// InvalidArgumentException. It is also the predicate that separates a PHP
// throwable from an error a Go binding returned, which a Go type name cannot
// do; a driver type called Error would otherwise be read as PHP's Error class
// and fall out of the `catch (Exception $e)` a script wrote around its query.
type Throwable interface {
	error

	// ThrowableClass returns the PHP class name, such as "InvalidArgumentException".
	ThrowableClass() string
}

// throwableClassOf returns the PHP class an error represents, and whether the
// error is a PHP throwable at all.
//
// An error that is not one is still catchable: a Go binding returning an error,
// and a panic recovered at the host boundary, reach a script through the same
// try/catch and belong to no PHP class. Reporting them as "not a throwable" is
// what lets matchCatchType offer them to any clause, which is the contract a
// host binding is written against.
func throwableClassOf(err error) (string, bool) {
	t, ok := err.(Throwable)
	if !ok {
		return "", false
	}
	class := t.ThrowableClass()
	return class, class != ""
}
