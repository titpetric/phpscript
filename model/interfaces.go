package model

import (
	"fmt"
	"strings"
)

// InterfaceViolation names a method an interface requires that a declaring
// class omits.
//
// Decl is the class declaration the violation came from. A caller holding the
// program's source spans can report the class's line.
type InterfaceViolation struct {
	Decl      *ClassDecl
	Class     string
	Interface string
	Method    string
}

// String names the class, the method it does not declare and the interface.
func (v InterfaceViolation) String() string {
	return fmt.Sprintf("class %s does not declare method %s() required by interface %s",
		v.Class, v.Method, v.Interface)
}

// InterfaceContractError is the failure both backends raise for a violated
// contract. It reaches a script as a RuntimeException. A `catch
// (RuntimeException $e)` written around an include takes it.
type InterfaceContractError struct {
	Violations []InterfaceViolation
}

// Error lists every violation, one on each line. A class missing three methods
// produces one error, not three.
func (e *InterfaceContractError) Error() string {
	parts := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		parts = append(parts, v.String())
	}
	return strings.Join(parts, "; ")
}
