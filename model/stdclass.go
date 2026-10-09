package model

// stdClass is PHP's empty class: it declares no property, no method and no
// constant, and exists so that a value with named parts has somewhere to live.
// `new stdClass` and the `(object)` cast both produce an instance of it.
//
// It is a package-level value and no entry in the runtime's class
// table for two reasons. The table is cleared between runs, so a seeded entry
// would have to be put back on every reset; and RegisterClass merges into an
// existing entry, so a script that declared its own `class stdClass` would
// write its methods onto the one every other instance shares. Kept here, the
// declaration shadows it in the table and this value is never reached.
var stdClass = &Class{Name: "stdClass"}

// StdClass returns the class every stdClass instance points at. All of them
// share it, so two of them are the same class and never two
// classes that happen to be spelled alike.
func StdClass() *Class {
	return stdClass
}

// NewStdClass builds an empty stdClass instance.
func NewStdClass() *Object {
	return NewObject(stdClass)
}
