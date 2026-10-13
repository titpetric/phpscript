package runner

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestResolveClassName covers the contextual class names a static reference
// accepts. `self` and `static` both name the class of the running method,
// because there is no inheritance and late static binding resolves to the same
// class; `parent` has nothing to resolve to. A leading separator comes off a
// written name.
func TestResolveClassName(t *testing.T) {
	inMethod := NewScope()
	inMethod.Set("__class__", "Store")

	for _, tc := range []struct {
		name  string
		class string
		scope *Scope
		want  string
	}{
		{"self inside a method", "self", inMethod, "Store"},
		{"static inside a method", "static", inMethod, "Store"},
		{"parent inside a method", "parent", inMethod, "Store"},
		{"self outside a method", "self", NewScope(), "self"},
		{"self with no scope", "self", nil, "self"},
		{"a written name", `\App\Store`, inMethod, "App\\Store"},
		{"a plain name", "Store", inMethod, "Store"},
	} {
		if got := resolveClassName(tc.class, tc.scope); got != tc.want {
			t.Errorf("%s: resolveClassName() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestClassDefaultScope covers the scope a class's declared defaults are
// evaluated in. A default is written inside the class body, so `self::X` in one
// resolves to that class; the scope the `new` was written in knows nothing
// about it. __FILE__ and __DIR__ cross from the caller, so a default naming
// either reads the file it was declared in.
func TestClassDefaultScope(t *testing.T) {
	rt := New(io.Discard, Options{})
	caller := NewScope()
	caller.Set("__FILE__", "/app/store.php")
	caller.Set("__DIR__", "/app")
	caller.Set("rows", int64(3))

	scope := rt.classDefaultScope(&model.Class{Name: "Store"}, caller)
	if value, _ := scope.Get("__class__"); value != "Store" {
		t.Errorf("__class__ = %#v, want the class name", value)
	}
	if value, _ := scope.Get("__FILE__"); value != "/app/store.php" {
		t.Errorf("__FILE__ = %#v, want the caller's file", value)
	}
	if value, _ := scope.Get("__DIR__"); value != "/app" {
		t.Errorf("__DIR__ = %#v, want the caller's directory", value)
	}
	if _, ok := scope.Get("rows"); ok {
		t.Error("a caller-local variable crossed into the defaults scope")
	}

	bare := rt.classDefaultScope(&model.Class{Name: "Store"}, nil)
	if value, _ := bare.Get("__class__"); value != "Store" {
		t.Errorf("__class__ = %#v with no caller, want the class name", value)
	}
}

// TestStaticBag covers the live storage of a class's static properties, seeded
// from the declared defaults on first access. One bag per class on the Runtime,
// so composer's `self::$registeredLoaders` is the same storage every instance
// and every later static call reads.
func TestStaticBag(t *testing.T) {
	rt := New(io.Discard, Options{})
	class := &model.Class{
		Name: "Loader",
		Statics: []model.Field{
			{Name: "registered", Default: &model.Lit{Value: int64(0)}},
			{Name: "prefix"},
		},
	}

	bag, err := rt.staticBag(class, NewScope())
	if err != nil {
		t.Fatalf("staticBag failed: %v", err)
	}
	if bag["registered"] != int64(0) {
		t.Errorf("the seeded default is %#v, want 0", bag["registered"])
	}
	if value, ok := bag["prefix"]; !ok || value != nil {
		t.Errorf("a static with no default is %#v, want nil", value)
	}

	bag["registered"] = int64(2)
	again, err := rt.staticBag(class, NewScope())
	if err != nil {
		t.Fatalf("the second staticBag failed: %v", err)
	}
	if again["registered"] != int64(2) {
		t.Errorf("the second access reseeded the bag: %#v", again["registered"])
	}
}

// TestStaticProp covers reading and writing `Class::$name`. A property that was
// never declared reads as null, the leniency the rest of the runtime applies to
// property and index access, and a write through the class name is visible to
// the next read.
func TestStaticProp(t *testing.T) {
	rt := New(io.Discard, Options{})
	class := &model.Class{
		Name:    "Loader",
		Statics: []model.Field{{Name: "registered", Default: &model.Lit{Value: int64(0)}}},
	}
	rt.RegisterClass(class)

	scope := NewScope()
	read := rt.helperStaticProp(&scopeRef{scope: scope})
	value, err := read("Loader", "registered")
	if err != nil {
		t.Fatalf("reading a static failed: %v", err)
	}
	if value != int64(0) {
		t.Errorf("Loader::$registered = %#v, want 0", value)
	}

	if err := rt.setStaticProp("Loader", "registered", int64(3), scope); err != nil {
		t.Fatalf("writing a static failed: %v", err)
	}
	if value, _ := read("Loader", "registered"); value != int64(3) {
		t.Errorf("Loader::$registered = %#v after the write, want 3", value)
	}

	undeclared, err := read("Loader", "absent")
	if err != nil {
		t.Fatalf("reading an undeclared static failed: %v", err)
	}
	if undeclared != nil {
		t.Errorf("Loader::$absent = %#v, want nil", undeclared)
	}

	if _, err := read("Missing", "registered"); err == nil {
		t.Error("reading a static on an unknown class reported no error")
	}
}

// TestStaticVarBag covers the persistent storage of one `static $x` statement.
// Named functions and methods share one bag per statement on the Runtime; a
// closure frame carries its own table, so each closure value counts on its own.
// The second return reports whether the bag already held values, and keeps an
// initializer from running twice.
func TestStaticVarBag(t *testing.T) {
	rt := New(io.Discard, Options{})
	node := &model.StaticVar{Vars: []model.StaticVarDecl{{Name: "count"}}}

	bag, seeded := rt.staticVarBag(node, NewScope())
	if seeded {
		t.Error("the first access reported the bag as already seeded")
	}
	bag["count"] = int64(1)

	again, seeded := rt.staticVarBag(node, NewScope())
	if !seeded {
		t.Error("the second access reported the bag as unseeded")
	}
	if again["count"] != int64(1) {
		t.Errorf("the second access answered %#v, want the stored 1", again["count"])
	}

	closureFrame := NewScope()
	closureFrame.Set(staticsKey, map[*model.StaticVar]map[string]any{})
	own, seeded := rt.staticVarBag(node, closureFrame)
	if seeded {
		t.Error("a closure frame read the runtime's bag as seeded")
	}
	if own["count"] != nil {
		t.Errorf("a closure frame holds %#v, want its own empty bag", own["count"])
	}
}

// TestExecStaticVar covers `static $x = expr;`. The initializer runs only the
// first time the declaring function executes the statement; every call after
// that rebinds the name to the value the last call left behind, so a counter
// keeps counting.
func TestExecStaticVar(t *testing.T) {
	rt := New(io.Discard, Options{})
	node := &model.StaticVar{Vars: []model.StaticVarDecl{{Name: "count", Default: &model.Lit{Value: int64(0)}}}}

	first := NewScope()
	if err := rt.execStaticVar(node, first); err != nil {
		t.Fatalf("execStaticVar failed: %v", err)
	}
	if value, _ := first.Get("count"); value != int64(0) {
		t.Errorf("the first call reads %#v, want the initializer's 0", value)
	}
	first.Set("count", int64(1))

	second := NewScope()
	if err := rt.execStaticVar(node, second); err != nil {
		t.Fatalf("the second execStaticVar failed: %v", err)
	}
	if value, _ := second.Get("count"); value != int64(1) {
		t.Errorf("the second call reads %#v, want the 1 the first left behind", value)
	}
}
