package model

// stdClass is PHP's empty class: it declares no property, no method and no
// constant, and exists so that a value with named parts has somewhere to live.
// `new stdClass` and the `(object)` cast both produce an instance of it.
//
// It lives as a package-level value and no entry in the runtime's class table
// for two reasons. The runtime clears that table between runs, so every reset
// would have to seed the entry again; and RegisterClass merges into an
// existing entry, so a script that declared its own `class stdClass` would
// write its methods onto the one every other instance shares. Kept here, the
// declaration shadows it in the table and this value is never reached.
var stdClass = &Class{Name: "stdClass"}

// StdClass returns the class every stdClass instance points at. Every one
// shares it, so two instances are the same class and never two classes
// sharing a name.
func StdClass() *Class {
	return stdClass
}

// NewStdClass builds an empty stdClass instance.
func NewStdClass() *Object {
	return NewObject(stdClass)
}
