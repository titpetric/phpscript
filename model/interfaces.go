package model

import (
	"fmt"
	"strings"
)

// InterfaceViolation is one method an interface names and a class declaring
// `implements` did not declare.
//
// Decl is the class declaration it was found on, so a caller holding the
// program's source spans can report the line the class was written on.
type InterfaceViolation struct {
	Decl      *ClassDecl
	Class     string
	Interface string
	Method    string
}

func (v InterfaceViolation) String() string {
	return fmt.Sprintf("class %s does not declare method %s() required by interface %s",
		v.Class, v.Method, v.Interface)
}

// InterfaceContractError is the failure both backends raise for a violated
// contract. It reaches a script as a RuntimeException, so a `catch
// (RuntimeException $e)` written around an include takes it.
type InterfaceContractError struct {
	Violations []InterfaceViolation
}

func (e *InterfaceContractError) Error() string {
	parts := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		parts = append(parts, v.String())
	}
	return strings.Join(parts, "; ")
}

// CheckInterfaceContracts reports the contract violations in stmts as one
// error, or nil when every class declaring `implements` declares what it
// promised. Both backends call it where they register classes, so a program
// fails the same way whichever one runs it.
func CheckInterfaceContracts(prog *Program) error {
	violations := CheckInterfaces(prog.Stmts, prog.AnonClasses...)
	if len(violations) == 0 {
		return nil
	}
	return &InterfaceContractError{Violations: violations}
}

// CheckInterfaces checks every class in stmts that declares `implements`
// against the interfaces declared alongside it, and returns what is missing in
// declaration order.
//
// The check is a name comparison and nothing else: an interface names methods,
// and the class must declare each of them itself. Nothing is copied onto the
// class, so a class that passes the check has exactly the members it wrote.
//
// A name no `interface` declaration in stmts defines is skipped rather than
// reported. It is either a PHP built-in such as Countable, which phpscript does
// not declare, or an interface declared in a file that is not part of this
// statement list; neither is a contract this program can be held to.
func CheckInterfaces(stmts []Stmt, extra ...*ClassDecl) []InterfaceViolation {
	interfaces := interfaceIndex(stmts)
	if len(interfaces) == 0 {
		return nil
	}
	// extra carries the classes that are not statements: an anonymous class is
	// declared inside an expression, and says `implements` like any other.
	decls := make([]*ClassDecl, 0, len(stmts)+len(extra))
	for _, s := range stmts {
		if cd, ok := s.(*ClassDecl); ok {
			decls = append(decls, cd)
		}
	}
	decls = append(decls, extra...)

	var out []InterfaceViolation
	for _, cd := range decls {
		if len(cd.Implements) == 0 {
			continue
		}
		declared := declaredMethods(cd, stmts)
		for _, name := range cd.Implements {
			for _, req := range contractOf(name, interfaces) {
				if _, ok := declared[strings.ToLower(req.Method)]; ok {
					continue
				}
				out = append(out, InterfaceViolation{
					Decl:      cd,
					Class:     cd.Name,
					Interface: req.Interface,
					Method:    req.Method,
				})
			}
		}
	}
	return out
}

// interfaceIndex maps the lower-cased name of every interface declared in
// stmts to its declaration. PHP compares a class or interface name without
// regard to case, so the index is keyed that way.
func interfaceIndex(stmts []Stmt) map[string]*InterfaceDecl {
	var index map[string]*InterfaceDecl
	for _, s := range stmts {
		id, ok := s.(*InterfaceDecl)
		if !ok {
			continue
		}
		if index == nil {
			index = make(map[string]*InterfaceDecl, 4)
		}
		index[strings.ToLower(id.Name)] = id
	}
	return index
}

// requirement is one method name a contract asks for, and the interface that
// named it, which is what a violation reports.
type requirement struct {
	Interface string
	Method    string
}

// contractOf returns the methods named by an interface and, transitively, by
// every interface it extends. The result is a union of names computed here
// rather than a set of members held anywhere: an extended interface contributes
// what it declares to what the class must declare, and nothing else.
func contractOf(name string, index map[string]*InterfaceDecl) []requirement {
	var (
		out  []requirement
		seen = map[string]bool{}
	)
	collectContract(name, index, map[string]bool{}, seen, &out)
	return out
}

func collectContract(name string, index map[string]*InterfaceDecl, visited, seen map[string]bool, out *[]requirement) {
	key := strings.ToLower(name)
	if visited[key] {
		// `interface A extends B` and `interface B extends A` is not a PHP
		// program, but a parser accepts what it is given and this walk must
		// still terminate.
		return
	}
	visited[key] = true
	decl, ok := index[key]
	if !ok {
		return
	}
	for _, m := range decl.Methods {
		method := strings.ToLower(m.Name)
		if seen[method] {
			continue
		}
		seen[method] = true
		*out = append(*out, requirement{Interface: decl.Name, Method: m.Name})
	}
	for _, parent := range decl.Extends {
		collectContract(parent, index, visited, seen, out)
	}
}

// declaredMethods is the set of method names a class declares, lower-cased.
// Both spellings count: a method written in the class body, and one written at
// the top level as `function Class::method()`, which the runtime attaches to
// the same class.
func declaredMethods(cd *ClassDecl, stmts []Stmt) map[string]bool {
	declared := make(map[string]bool, len(cd.Methods))
	for _, m := range cd.Methods {
		declared[strings.ToLower(m.Name)] = true
	}
	for _, s := range stmts {
		fd, ok := s.(*FuncDecl)
		if !ok || fd.Class == "" {
			continue
		}
		if strings.EqualFold(fd.Class, cd.Name) {
			declared[strings.ToLower(fd.Name)] = true
		}
	}
	return declared
}

// InterfaceNames returns every interface name a class declares, plus the names
// those interfaces extend, lower-cased and deduplicated.
//
// It is the same union contractOf walks, taken as names rather than as methods,
// and it is what `instanceof` answers an interface name from. Nothing is
// inherited through it: the class holds the list it declared, and the list says
// which contracts it was checked against.
func InterfaceNames(cd *ClassDecl, stmts []Stmt) []string {
	if cd == nil || len(cd.Implements) == 0 {
		return nil
	}
	index := interfaceIndex(stmts)
	seen := map[string]bool{}
	var out []string
	var walk func(name string)
	walk = func(name string) {
		key := strings.ToLower(name)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, key)
		if id, ok := index[key]; ok {
			for _, parent := range id.Extends {
				walk(parent)
			}
		}
	}
	for _, name := range cd.Implements {
		walk(name)
	}
	return out
}
