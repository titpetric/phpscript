package model_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

func iface(name string, extends []string, methods ...string) *model.InterfaceDecl {
	decl := &model.InterfaceDecl{Name: name, Extends: extends}
	for _, m := range methods {
		decl.Methods = append(decl.Methods, &model.FuncDecl{Name: m})
	}
	return decl
}

func class(name string, implements []string, methods ...string) *model.ClassDecl {
	decl := &model.ClassDecl{Name: name, Implements: implements}
	for _, m := range methods {
		decl.Methods = append(decl.Methods, &model.FuncDecl{Name: m})
	}
	return decl
}

// TestCheckInterfaces pins what an interface is: a list of names a class has to
// declare itself. Nothing here inherits, so the only question ever asked is
// whether a name is present.
func TestCheckInterfaces(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		stmts []model.Stmt
		want  []string
	}{
		{
			name:  "satisfied contract",
			stmts: []model.Stmt{iface("R", nil, "get", "has"), class("S", []string{"R"}, "get", "has")},
		},
		{
			name:  "missing method",
			stmts: []model.Stmt{iface("R", nil, "get", "has"), class("S", []string{"R"}, "get")},
			want:  []string{"class S does not declare method has() required by interface R"},
		},
		{
			name: "every listed interface is checked",
			stmts: []model.Stmt{
				iface("R", nil, "get"), iface("W", nil, "put"),
				class("S", []string{"R", "W"}),
			},
			want: []string{
				"class S does not declare method get() required by interface R",
				"class S does not declare method put() required by interface W",
			},
		},
		{
			name: "an extended interface widens the contract",
			stmts: []model.Stmt{
				iface("R", nil, "get"), iface("L", []string{"R"}, "keys"),
				class("S", []string{"L"}, "keys"),
			},
			want: []string{"class S does not declare method get() required by interface R"},
		},
		{
			name:  "an undeclared interface name is not a contract",
			stmts: []model.Stmt{iface("R", nil, "get"), class("S", []string{"Countable"})},
		},
		{
			name:  "method and interface names are compared without case",
			stmts: []model.Stmt{iface("R", nil, "Get"), class("S", []string{"r"}, "gEt")},
		},
		{
			name: "a method declared as function Class::method counts",
			stmts: []model.Stmt{
				iface("R", nil, "get"), class("S", []string{"R"}),
				&model.FuncDecl{Class: "S", Name: "get"},
			},
		},
		{
			name:  "a cycle in extends terminates",
			stmts: []model.Stmt{iface("A", []string{"B"}, "a"), iface("B", []string{"A"}, "b"), class("S", []string{"A"})},
			want: []string{
				"class S does not declare method a() required by interface A",
				"class S does not declare method b() required by interface B",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := model.CheckInterfaces(tc.stmts)
			if len(got) != len(tc.want) {
				t.Fatalf("violations = %v, want %v", got, tc.want)
			}
			for i, v := range got {
				if v.String() != tc.want[i] {
					t.Errorf("violation[%d] = %q, want %q", i, v.String(), tc.want[i])
				}
				if v.Decl == nil {
					t.Errorf("violation[%d] carries no class declaration", i)
				}
			}
			err := model.CheckInterfaceContracts(&model.Program{Stmts: tc.stmts})
			if len(tc.want) == 0 {
				if err != nil {
					t.Fatalf("CheckInterfaceContracts = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("CheckInterfaceContracts = nil, want an error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not report %q", err, want)
				}
			}
		})
	}
}

// TestInterfaceContractError covers the type a caller recovers the violations
// through. Both backends raise it where they register classes, so a program
// fails the same way whichever one ran it, and a caller holding the program's
// source spans reads Decl to report the class's line.
func TestInterfaceContractError(t *testing.T) {
	t.Parallel()
	stmts := []model.Stmt{iface("R", nil, "get", "has"), class("S", []string{"R"}, "get")}

	err := model.CheckInterfaceContracts(&model.Program{Stmts: stmts})
	if err == nil {
		t.Fatal("CheckInterfaceContracts answered nil for a violated contract")
	}

	var contract *model.InterfaceContractError
	if !errors.As(err, &contract) {
		t.Fatalf("CheckInterfaceContracts answered %T, want *model.InterfaceContractError", err)
	}
	if got := len(contract.Violations); got != 1 {
		t.Fatalf("the error carries %d violations, want 1", got)
	}

	violation := contract.Violations[0]
	if violation.Method != "has" {
		t.Errorf("the violation names method %q, want \"has\"", violation.Method)
	}
	if violation.Class != "S" || violation.Interface != "R" {
		t.Errorf("the violation names class %q and interface %q, want \"S\" and \"R\"", violation.Class, violation.Interface)
	}
	if violation.Decl == nil {
		t.Fatal("the violation carries no class declaration, so a caller cannot name the line")
	}
	if violation.Decl.Name != "S" {
		t.Errorf("the declaration is for class %q, want \"S\"", violation.Decl.Name)
	}
}

// TestInterfaceContractError_Error covers the message a class missing several
// methods is reported through. The violations are joined into one string, so
// three missing methods are one error and no three.
func TestInterfaceContractError_Error(t *testing.T) {
	t.Parallel()
	err := &model.InterfaceContractError{Violations: []model.InterfaceViolation{
		{Class: "S", Interface: "R", Method: "get"},
		{Class: "S", Interface: "R", Method: "has"},
	}}
	want := "class S does not declare method get() required by interface R; " +
		"class S does not declare method has() required by interface R"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	if got := (&model.InterfaceContractError{}).Error(); got != "" {
		t.Errorf("Error() with no violations = %q, want the empty string", got)
	}
}

// TestInterfaceNames covers the names `instanceof` answers an interface name
// from: what the class declared, plus what those interfaces extend, lower-cased
// and deduplicated. PHP compares a class name without regard to case, so the
// list is keyed that way.
//
// A name no interface declaration defines is listed all the same. Such a class
// is checked against nothing, because the name is a php built-in this runtime
// does not declare, and `$a instanceof Countable` still answers what the author
// declared.
func TestInterfaceNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		decl  *model.ClassDecl
		stmts []model.Stmt
		want  []string
	}{
		{
			name: "no class",
		},
		{
			name: "a class declaring nothing",
			decl: class("S", nil),
		},
		{
			name:  "one interface",
			decl:  class("S", []string{"R"}),
			stmts: []model.Stmt{iface("R", nil, "get")},
			want:  []string{"r"},
		},
		{
			name:  "an extended interface widens the list",
			decl:  class("S", []string{"L"}),
			stmts: []model.Stmt{iface("R", nil, "get"), iface("L", []string{"R"}, "keys")},
			want:  []string{"l", "r"},
		},
		{
			name:  "an undeclared name is listed",
			decl:  class("S", []string{"Countable"}),
			stmts: []model.Stmt{iface("R", nil, "get")},
			want:  []string{"countable"},
		},
		{
			name:  "two spellings of one name collapse",
			decl:  class("S", []string{"R", "r"}),
			stmts: []model.Stmt{iface("R", nil, "get")},
			want:  []string{"r"},
		},
		{
			name:  "a cycle in extends terminates",
			decl:  class("S", []string{"A"}),
			stmts: []model.Stmt{iface("A", []string{"B"}, "a"), iface("B", []string{"A"}, "b")},
			want:  []string{"a", "b"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := model.InterfaceNames(tc.decl, tc.stmts)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("InterfaceNames() = %v, want %v", got, tc.want)
			}
		})
	}
}
