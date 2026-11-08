package model

import (
	"errors"
	"fmt"
	"strings"
)

// This file holds the structural check each declaration type declares. A parser
// fills these types from source and a host fills them from a decoded request,
// a generated registration or a hand-written table, where nothing has read the
// grammar: Valid is the check those callers get in place of one.
//
// Each reports the first fault it finds and names the declaration. None of
// them reads a body, resolves a name against a program or reaches another
// declaration: modelutil holds the checks that need the whole tree.

// errNoName is the fault every declaration here shares.
var errNoName = errors.New("declares no name")

// Valid reports a parameter naming something and declaring one default form.
func (p Param) Valid() error {
	if p.Name == "" {
		return fmt.Errorf("parameter: %w", errNoName)
	}
	if p.Variadic && p.Default != nil {
		return fmt.Errorf("parameter $%s is variadic and declares a default", p.Name)
	}
	return nil
}

// Valid reports a field naming something under a visibility php spells.
func (f Field) Valid() error {
	if f.Name == "" {
		return fmt.Errorf("property: %w", errNoName)
	}
	switch f.Visibility {
	case "", "public", "protected", "private":
		return nil
	}
	return fmt.Errorf("property $%s declares visibility %q, want public, protected or private", f.Name, f.Visibility)
}

// Valid reports a function declaring a name, a parameter list php can call and
// a body matching what it promised.
//
// An abstract declaration holds no body and a concrete one holds one. The
// variadic parameter is last, because a parameter after it takes nothing.
func (d *FuncDecl) Valid() error {
	if d == nil {
		return errors.New("function: nil declaration")
	}
	if d.Name == "" {
		return fmt.Errorf("function: %w", errNoName)
	}
	label := d.Name
	if d.Class != "" {
		label = d.Class + "::" + d.Name
	}
	if d.Abstract && len(d.Body) > 0 {
		return fmt.Errorf("function %s is abstract and declares a body", label)
	}
	seen := make(map[string]bool, len(d.Params))
	for i, param := range d.Params {
		if err := param.Valid(); err != nil {
			return fmt.Errorf("function %s: %w", label, err)
		}
		if seen[param.Name] {
			return fmt.Errorf("function %s declares $%s twice", label, param.Name)
		}
		seen[param.Name] = true
		if param.Variadic && i != len(d.Params)-1 {
			return fmt.Errorf("function %s declares $%s variadic ahead of another parameter", label, param.Name)
		}
	}
	return nil
}

// Valid reports a class declaring a name and a member list with no repeated
// name, over its properties, its statics, its constants and its methods.
//
// It reads Implements as names alone; modelutil.CheckInterfaces resolves them
// against the interfaces a program declares.
func (d *ClassDecl) Valid() error {
	if d == nil {
		return errors.New("class: nil declaration")
	}
	if d.Name == "" {
		return fmt.Errorf("class: %w", errNoName)
	}
	for _, group := range []struct {
		kind   string
		fields []Field
	}{
		{"property", d.Fields},
		{"static property", d.Statics},
		{"constant", d.Consts},
	} {
		if err := validFields(d.Name, group.kind, group.fields); err != nil {
			return err
		}
	}
	seen := make(map[string]bool, len(d.Methods))
	for _, method := range d.Methods {
		if err := method.Valid(); err != nil {
			return fmt.Errorf("class %s: %w", d.Name, err)
		}
		lower := strings.ToLower(method.Name)
		if seen[lower] {
			return fmt.Errorf("class %s declares method %s() twice", d.Name, method.Name)
		}
		seen[lower] = true
	}
	return nil
}

// Reports the first repeated or malformed name of one member group.
func validFields(class, kind string, fields []Field) error {
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if err := field.Valid(); err != nil {
			return fmt.Errorf("class %s: %w", class, err)
		}
		if seen[field.Name] {
			return fmt.Errorf("class %s declares %s $%s twice", class, kind, field.Name)
		}
		seen[field.Name] = true
	}
	return nil
}

// Valid reports a resolved class naming something and holding no nil method.
func (c *Class) Valid() error {
	if c == nil {
		return errors.New("class: nil")
	}
	if c.Name == "" {
		return fmt.Errorf("class: %w", errNoName)
	}
	for name, method := range c.Methods {
		if method == nil {
			return fmt.Errorf("class %s holds a nil declaration under method %s()", c.Name, name)
		}
	}
	return validFields(c.Name, "property", c.Fields)
}

// Valid reports a callable holding exactly one of its two forms.
// A Func with neither set is a name a script can call and nothing runs.
func (f Func) Valid() error {
	switch {
	case f.Decl == nil && f.Go == nil:
		return errors.New("callable: holds neither a declaration nor a go func")
	case f.Decl != nil && f.Go != nil:
		return errors.New("callable: holds both a declaration and a go func")
	case f.Decl != nil:
		return f.Decl.Valid()
	}
	return nil
}

// Valid reports an instance pointing at a class, the one path to its methods
// and its property order.
func (o *Object) Valid() error {
	if o == nil {
		return errors.New("object: nil")
	}
	if o.Class == nil {
		return errors.New("object: holds no class")
	}
	return o.Class.Valid()
}
