package runner

// phpscript has no OOP. There is no class hierarchy, no inheritance and no
// `extends` at runtime.
//
// A catch clause matches on the name a throwable records, by the rule in
// matchCatchType, and `instanceof` is class-name equality. The parser reads
// `extends` and records it on model.ClassDecl so the formatter can print it
// back and the linter can see it; nothing in runner may read it.
//
// Do not add a parent table, a Parent field on model.Class, or a hierarchy
// walk. docs/design.md refuses that design.

// Throwable marks a value that records its PHP class.
//
// ThrowableClass reads the class off the value and not off the Go type: every
// SPL name is one Go type and reflection would answer "Exception" for an
// InvalidArgumentException. The method is also the predicate separating a PHP
// throwable from an error a Go binding returned, which a Go type name cannot
// do: it would take a driver type called Error for PHP's Error class, and the
// error would fall out of the `catch (Exception $e)` a script wrote.
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
// try/catch and belong to no PHP class. Reporting them as "not a throwable"
// lets matchCatchType route them to any clause, the contract every host
// binding relies on.
func throwableClassOf(err error) (string, bool) {
	t, ok := err.(Throwable)
	if !ok {
		return "", false
	}
	class := t.ThrowableClass()
	return class, class != ""
}
