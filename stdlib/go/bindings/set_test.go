package bindings_test

import (
	"io"
	"net/http"
	"net/mail"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/go/bindings"
)

// TestSet covers a set end to end on one runtime: a func binds as a function,
// a value binds as a constant, and a type a signature reaches binds as a
// class.
func TestSet(t *testing.T) {
	t.Parallel()
	set := bindings.For("net/http").
		Bind("NewRequest", http.NewRequest).
		Bind("StatusOK", http.StatusOK)
	rt := runner.New(io.Discard, runner.Options{})
	set.Register(rt)
	if !rt.FunctionExists(`Runtime\Go\Net\HTTP\NewRequest`) {
		t.Error("NewRequest is undefined")
	}
	if v, ok := rt.Const(`Runtime\Go\Net\HTTP\StatusOK`); !ok || v != http.StatusOK {
		t.Errorf("StatusOK reads %v, %v", v, ok)
	}
	if _, ok := rt.LookupConstructor(`Runtime\Go\Net\HTTP\Request`); !ok {
		t.Error("the type the signature reaches is no class")
	}
}

// TestSet_Bind covers the order the entries keep: the one a binding wrote.
func TestSet_Bind(t *testing.T) {
	t.Parallel()
	set := bindings.For("net/http").Bind("Get", http.Get).Bind("Post", http.Post)
	if got := set.Entries(); got[0].Name != "Get" || got[1].Name != "Post" {
		t.Errorf("the entries read %s, %s", got[0].Name, got[1].Name)
	}
}

// TestSet_Entries covers a set that binds nothing, which registers nothing.
func TestSet_Entries(t *testing.T) {
	t.Parallel()
	if got := bindings.For("strings").Entries(); len(got) != 0 {
		t.Errorf("an empty set holds %d entries", len(got))
	}
}

// TestSet_Area covers the area each path opens, including the acronyms the
// areas spell their own way.
func TestSet_Area(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]string{
		"strings":       `Runtime\Go\Strings`,
		"encoding/json": `Runtime\Go\Encoding\JSON`,
		"database/sql":  `Runtime\Go\Database\SQL`,
	} {
		if got := bindings.For(path).Area(); got != want {
			t.Errorf("%s opens %s, want %s", path, got, want)
		}
	}
}

// TestSet_Install covers the contribution an import resolves: the installer
// lands under the area, where a runtime finds it by name.
func TestSet_Install(t *testing.T) {
	bindings.For("net/mail").Bind("ParseAddress", mail.ParseAddress).Install()
	if _, ok := runner.NamespaceInstaller(`Runtime\Go\Net\Mail`); !ok {
		t.Error("the area is not registered")
	}
}

// TestSet_Register covers a second Register over one runtime, which an import
// of the same package from two files reaches.
func TestSet_Register(t *testing.T) {
	t.Parallel()
	set := bindings.For("net/http").Bind("Get", http.Get)
	rt := runner.New(io.Discard, runner.Options{})
	set.Register(rt)
	set.Register(rt)
	if !rt.FunctionExists(`Runtime\Go\Net\HTTP\Get`) {
		t.Error("Get is undefined")
	}
}
