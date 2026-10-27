package runner

import (
	"errors"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestBindParams covers the binding a callee's frame starts from: positional
// arguments by name, a default evaluated for a parameter the caller skipped,
// nil for one with no default, and a variadic tail collected into one array,
// an empty one when the caller stopped at it.
func TestBindParams(t *testing.T) {
	t.Parallel()
	rt := New(nil, Options{})
	plain := &model.FuncDecl{Params: []model.Param{
		{Name: "name"},
		{Name: "count", Default: &model.Lit{Value: int64(3)}},
		{Name: "plain"},
	}}

	scope := NewScope()
	if err := rt.bindParams(plain, []any{"ada"}, scope); err != nil {
		t.Fatalf("bindParams: %v", err)
	}
	if v, _ := scope.Get("name"); v != "ada" {
		t.Errorf("name = %#v, want the positional argument", v)
	}
	if v, _ := scope.Get("count"); v != int64(3) {
		t.Errorf("count = %#v, want the evaluated default", v)
	}
	if v, ok := scope.Get("plain"); !ok || v != nil {
		t.Errorf("plain = %#v, %v, want nil bound for a skipped parameter", v, ok)
	}

	variadic := &model.FuncDecl{Params: []model.Param{
		{Name: "name"},
		{Name: "rest", Variadic: true},
	}}

	stopped := NewScope()
	if err := rt.bindParams(variadic, []any{"ada"}, stopped); err != nil {
		t.Fatalf("bindParams: %v", err)
	}
	rest, _ := stopped.Get("rest")
	if arr, ok := rest.(*model.Array); !ok || arr.Len() != 0 {
		t.Errorf("rest = %#v, want an empty array when the caller stopped at it", rest)
	}

	// Short of the variadic parameter and not merely at it: two names before
	// `...$rest` and one argument leaves the parameter index past the end of
	// args, which sliced from there panics on the bound. `f($a, $b, ...$rest)`
	// called as `f("x")` is the shape, and it reached the caller as a host
	// panic reading `slice bounds out of range [2:1]`.
	short := &model.FuncDecl{Params: []model.Param{
		{Name: "name"},
		{Name: "second"},
		{Name: "rest", Variadic: true},
	}}
	skipped := NewScope()
	if err := rt.bindParams(short, []any{"ada"}, skipped); err != nil {
		t.Fatalf("bindParams past the end: %v", err)
	}
	if v, ok := skipped.Get("second"); !ok || v != nil {
		t.Errorf("second = %#v, %v, want nil bound for the parameter the caller skipped", v, ok)
	}
	past, _ := skipped.Get("rest")
	if arr, ok := past.(*model.Array); !ok || arr.Len() != 0 {
		t.Errorf("rest = %#v, want an empty array when the caller stopped before it", past)
	}

	full := NewScope()
	if err := rt.bindParams(variadic, []any{"ada", "a", "b"}, full); err != nil {
		t.Fatalf("bindParams: %v", err)
	}
	rest, _ = full.Get("rest")
	arr, ok := rest.(*model.Array)
	if !ok || arr.Len() != 2 {
		t.Fatalf("rest = %#v, want the two trailing arguments", rest)
	}
	if first, _ := arr.Get(int64(0)); first != "a" {
		t.Errorf("rest[0] = %#v, want the first trailing argument", first)
	}
}

// TestCombineErrors covers what a frame's unwind reports. One error travels
// unchanged and errors.As still reaches its type; two or more travel
// together and a failing defer does not hide the failure that ran it.
func TestCombineErrors(t *testing.T) {
	t.Parallel()
	if err := combineErrors(nil, nil); err != nil {
		t.Errorf("combineErrors(nil, nil) = %v, want nil", err)
	}

	single := NewRuntimeException("no such invoice", 0)
	combined := combineErrors(nil, single, nil)
	if combined != error(single) {
		t.Errorf("one error came back as %v, want it unchanged", combined)
	}

	other := errors.New("the flush failed")
	both := combineErrors(single, other)
	if !errors.Is(both, single) || !errors.Is(both, other) {
		t.Errorf("combineErrors kept %v, want both errors reachable", both)
	}
}

// TestInvokeClosureCaptures covers the frame a closure runs in: the `use`
// captures are a snapshot, the enclosing frame's other variables are out of
// reach, and a parameter of the same name wins over a capture.
func TestInvokeClosureCaptures(t *testing.T) {
	t.Parallel()
	rt := New(nil, Options{})

	cl := &model.Closure{
		Params: []model.Param{{Name: "name"}},
		Uses:   []model.ClosureUse{{Name: "greeting"}, {Name: "name"}},
		Body: []model.Stmt{
			&model.Return{Value: &model.Binary{
				Op:    ".",
				Left:  &model.Var{Name: "greeting"},
				Right: &model.Binary{Op: ".", Left: &model.Var{Name: "name"}, Right: &model.Var{Name: "hidden"}},
			}},
		},
	}

	defining := NewScope()
	defining.Set("greeting", "hello ")
	defining.Set("name", "captured")
	defining.Set("hidden", "leaked")

	env := captureClosureEnv(cl, defining)
	value, err := rt.invokeClosure(cl, []any{"ada"}, env)
	if err != nil {
		t.Fatalf("invokeClosure: %v", err)
	}
	// The parameter wins over the capture of the same name, and the enclosing
	// frame's unlisted variable is unreachable and it reads as null.
	if value != "hello ada" {
		t.Errorf("the closure answered %#v, want %q", value, "hello ada")
	}
}
