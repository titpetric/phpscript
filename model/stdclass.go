package model

// stdClass is PHP's empty class, a package value a script may shadow.
var stdClass = &Class{Name: "stdClass"}

// StdClass returns the class every stdClass instance shares.
func StdClass() *Class {
	return stdClass
}

// NewStdClass builds an empty stdClass instance.
func NewStdClass() *Object {
	return NewObject(stdClass)
}
