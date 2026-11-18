package modelutil

import (
	"strings"

	"github.com/titpetric/phpscript/model"
)

// CheckInterfaceContracts reports the contract violations in prog as a single
// error, or nil when every class declaring `implements` keeps its promise.
func CheckInterfaceContracts(prog *model.Program) error {
	violations := CheckInterfaces(prog.Stmts, prog.AnonClasses...)
	if len(violations) == 0 {
		return nil
	}
	return &model.InterfaceContractError{Violations: violations}
}

// CheckInterfaces checks every class in stmts that declares `implements`
// against the interfaces declared alongside it. It returns what is missing in
// declaration order. The check compares names and copies nothing onto a class.
//
// CheckInterfaces skips a name no interface declaration in stmts defines: a
// php built-in this runtime does not declare, or an interface from another
// file. Such a name is no contract this program must meet.
func CheckInterfaces(stmts []model.Stmt, extra ...*model.ClassDecl) []model.InterfaceViolation {
	interfaces := interfaceIndex(stmts)
	if len(interfaces) == 0 {
		return nil
	}
	// extra holds the classes that are not statements: an anonymous class
	// sits inside an expression, and says `implements` like any other.
	decls := make([]*model.ClassDecl, 0, len(stmts)+len(extra))
	for _, s := range stmts {
		if cd, ok := s.(*model.ClassDecl); ok {
			decls = append(decls, cd)
		}
	}
	decls = append(decls, extra...)

	var out []model.InterfaceViolation
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
				out = append(out, model.InterfaceViolation{
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

// interfaceIndex maps each lower-cased interface name in stmts to its declaration.
func interfaceIndex(stmts []model.Stmt) map[string]*model.InterfaceDecl {
	var index map[string]*model.InterfaceDecl
	for _, s := range stmts {
		id, ok := s.(*model.InterfaceDecl)
		if !ok {
			continue
		}
		if index == nil {
			index = make(map[string]*model.InterfaceDecl, 4)
		}
		index[strings.ToLower(id.Name)] = id
	}
	return index
}

// contractMethod is a method name a contract declares, with its interface.
type contractMethod struct {
	Interface string
	Method    string
}

// contractOf returns the method names an interface and its extended interfaces require.
func contractOf(name string, index map[string]*model.InterfaceDecl) []contractMethod {
	var (
		out  []contractMethod
		seen = map[string]bool{}
	)
	collectContract(name, index, map[string]bool{}, seen, &out)
	return out
}

func collectContract(name string, index map[string]*model.InterfaceDecl, visited, seen map[string]bool, out *[]contractMethod) {
	key := strings.ToLower(name)
	if visited[key] {
		// `interface A extends B` and `interface B extends A` is no PHP
		// program, but a parser accepts whatever the source says and this
		// walk must still end.
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
		*out = append(*out, contractMethod{Interface: decl.Name, Method: m.Name})
	}
	for _, parent := range decl.Extends {
		collectContract(parent, index, visited, seen, out)
	}
}

// declaredMethods returns the lower-cased names a class declares, body or top level.
func declaredMethods(cd *model.ClassDecl, stmts []model.Stmt) map[string]bool {
	declared := make(map[string]bool, len(cd.Methods))
	for _, m := range cd.Methods {
		declared[strings.ToLower(m.Name)] = true
	}
	for _, s := range stmts {
		fd, ok := s.(*model.FuncDecl)
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
// The result matches the union contractOf walks, taken as names, not as
// methods. `instanceof` answers an interface name from the result. A class
// inherits nothing from the list: the class holds the names it declared.
// That list says which contracts it checks against.
func InterfaceNames(cd *model.ClassDecl, stmts []model.Stmt) []string {
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
