package model

// stdClass is PHP's empty class, kept as a package value so a script may shadow it.
var stdClass = &Class{Name: "stdClass"}

// StdClass returns the class every stdClass instance shares.
func StdClass() *Class {
	return stdClass
}

// NewStdClass builds an empty stdClass instance.
func NewStdClass() *Object {
	return NewObject(stdClass)
}
