package model_test

import (
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
			err := model.CheckInterfaceContracts(tc.stmts)
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
