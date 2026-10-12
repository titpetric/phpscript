package model

// Class is the resolved, runnable form of a ClassDecl: field defaults plus a
// method table keyed by method name.
//
// Statics are the declarations of `static $name = expr` properties; their
// values live in the runtime (one bag per class, created on first access) so
// that every instance and every static call observes the same storage.
type Class struct {
	Name string
	// Implements is every interface name the declaration listed, plus the names
	// those interfaces extend, lower-cased. It records which contracts the
	// class was checked against, and is what `instanceof` answers an interface
	// name from. No member arrives through it; see docs/design.md.
	Implements []string
	Fields     []Field
	Statics    []Field // static property declarations (Name + default Expr)
	Consts     []Field // class constants (Name + value Expr)
	Methods    map[string]*FuncDecl
}

// declares reports whether name is one of the class's declared fields.
func (c *Class) declares(name string) bool {
	for _, field := range c.Fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

// Field returns the declaration of the named field, for a caller that needs the
// visibility or type a property was declared with.
func (c *Class) Field(name string) (Field, bool) {
	for _, field := range c.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return Field{}, false
}
