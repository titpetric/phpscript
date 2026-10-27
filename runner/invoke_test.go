package runner

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/titpetric/phpscript/model"
)

// TestPlural covers the count-with-noun rendering PHP's argument count error
// spells, down to the singular: "1 argument" against "2 arguments".
func TestPlural(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		n    int
		want string
	}{
		{0, "0 arguments"},
		{1, "1 argument"},
		{2, "2 arguments"},
	} {
		if got := plural(tc.n, "argument"); got != tc.want {
			t.Errorf("plural(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// TestArgAt covers reading a declared parameter the script did not pass. PHP
// tolerates the short call and the missing argument is nil and the reflect path
// pads with the zero value the same way.
func TestArgAt(t *testing.T) {
	t.Parallel()
	args := []any{"first", int64(2)}
	if got := argAt(args, 0); got != "first" {
		t.Errorf("argAt(0) = %#v, want the first argument", got)
	}
	if got := argAt(args, 1); got != int64(2) {
		t.Errorf("argAt(1) = %#v, want the second argument", got)
	}
	if got := argAt(args, 2); got != nil {
		t.Errorf("argAt(2) = %#v past the end, want nil", got)
	}
	if got := argAt(nil, 0); got != nil {
		t.Errorf("argAt on no arguments = %#v, want nil", got)
	}
}

// TestArgsTail covers what follows the fixed parameters in a variadic call.
// argsTail aliases the slice and never copies it: the runner builds args fresh
// per call and the binding sees what reflect.Value.Call would have handed it.
func TestArgsTail(t *testing.T) {
	t.Parallel()
	args := []any{"a", "b", "c"}

	first := argsTail(args)
	if len(first) != 2 || first[0] != "b" {
		t.Errorf("argsTail() = %#v, want everything after the first argument", first)
	}
	second := argsTail2(args)
	if len(second) != 1 || second[0] != "c" {
		t.Errorf("argsTail2() = %#v, want everything after the second argument", second)
	}

	if got := argsTail([]any{"a"}); got != nil {
		t.Errorf("argsTail() with one argument = %#v, want nothing", got)
	}
	if got := argsTail2([]any{"a", "b"}); got != nil {
		t.Errorf("argsTail2() with two arguments = %#v, want nothing", got)
	}

	first[0] = "edited"
	if args[1] != "edited" {
		t.Error("argsTail copied the arguments where it aliases them")
	}
}

// TestPHPParamTypeName covers the spelling a TypeError uses for a declared Go
// parameter ("must be of type int"). It describes a parameter, which keeps it
// separate from phpDebugType, which describes a value, and from gettype's
// legacy names in stdlib: neither table folds into the other.
func TestPHPParamTypeName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value any
		want  string
	}{
		{int(0), "int"},
		{int64(0), "int"},
		{uint8(0), "int"},
		{float32(0), "float"},
		{float64(0), "float"},
		{"", "string"},
		{false, "bool"},
		{[]string{}, "array"},
		{map[string]int{}, "array"},
		{(*model.Array)(nil), "array"},
		{func() {}, "callable"},
		{time.Duration(0), "int"},
	} {
		if got := phpParamTypeName(reflect.TypeOf(tc.value)); got != tc.want {
			t.Errorf("phpParamTypeName(%T) = %q, want %q", tc.value, got, tc.want)
		}
	}

	if got := phpParamTypeName(nil); got != "mixed" {
		t.Errorf("phpParamTypeName(nil) = %q, want %q", got, "mixed")
	}
	if got := phpParamTypeName(reflect.TypeOf((*error)(nil)).Elem()); got != "mixed" {
		t.Errorf("an interface parameter reads as %q, want %q", got, "mixed")
	}
}

// TestPHPDebugType covers the name get_debug_type() reports for a value. A
// callable value is Closure, the class php answers for a first-class callable,
// and an interpreted object is its declared class.
func TestPHPDebugType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"null", nil, "null"},
		{"a bool", true, "bool"},
		{"a string", "text", "string"},
		{"bytes", []byte("text"), "string"},
		{"an int64", int64(1), "int"},
		{"an int", 1, "int"},
		{"a float", 1.5, "float"},
		{"an array", model.NewArray(), "array"},
		{"an object", model.NewObject(&model.Class{Name: "Store"}), "Store"},
		{"an instance of no class", model.NewObject(nil), "object"},
		{"a callable", &Callable{}, "Closure"},
		{"a Go slice", []string{}, "array"},
		{"a Go map", map[string]int{}, "array"},
		{"a Go func", func() {}, "Closure"},
	} {
		if got := phpDebugType(tc.value); got != tc.want {
			t.Errorf("%s: phpDebugType() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestCoerceArg covers the conversion a binding's declared parameter takes. A
// string parameter renders the value the way PHP renders it in a string
// context, because Go's own conversion means something else: reflect turns
// int64(65) into "A" where PHP writes "65". A duration-taking parameter accepts
// Go's readable spelling and time.Time.Add takes "30m".
func TestCoerceArg(t *testing.T) {
	t.Parallel()
	stringType := reflect.TypeOf("")
	if got, ok := coerceArg(stringType, int64(65)); !ok || got.String() != "65" {
		t.Errorf("coerceArg(string, 65) = %q, want %q", got.String(), "65")
	}

	intType := reflect.TypeOf(int(0))
	if got, ok := coerceArg(intType, int64(7)); !ok || got.Int() != 7 {
		t.Errorf("coerceArg(int, int64(7)) = %v, want 7", got)
	}

	if got, ok := coerceArg(durationType, "30m"); !ok || got.Interface() != 30*time.Minute {
		t.Errorf("coerceArg(Duration, \"30m\") = %v, want 30m", got)
	}
	if _, ok := coerceArg(durationType, "not a duration"); ok {
		t.Error("an unparseable duration was accepted")
	}

	if got, ok := coerceArg(intType, nil); !ok || got.Int() != 0 {
		t.Errorf("coerceArg(int, nil) = %v, want the zero value", got)
	}
	if got, ok := coerceArg(nil, "text"); !ok || got.Interface() != "text" {
		t.Errorf("coerceArg with no declared type = %v, want the value", got)
	}
	if _, ok := coerceArg(intType, model.NewArray()); ok {
		t.Error("an array was accepted for an int parameter")
	}
}

// TestCallResult covers reducing a reflect Call result to a value and an error.
// A Go callable returning two or more values becomes a PHP list in declaration
// order and a script destructures it: keeping only the first would answer a
// different question, since time.Time.ISOWeek returns the year and the week.
func TestCallResult(t *testing.T) {
	t.Parallel()
	call := func(fn any, args ...any) (any, error) {
		in := make([]reflect.Value, len(args))
		for i, arg := range args {
			in[i] = reflect.ValueOf(arg)
		}
		return callResult(reflect.ValueOf(fn).Call(in))
	}

	value, err := call(func() (int64, int64) { return 2026, 42 })
	if err != nil {
		t.Fatalf("a two-value return failed: %v", err)
	}
	list, ok := value.(*model.Array)
	if !ok {
		t.Fatalf("a two-value return answered %T, want an array", value)
	}
	if first, _ := list.Get(int64(0)); first != int64(2026) {
		t.Errorf("the list holds %#v first, want 2026", first)
	}
	if second, _ := list.Get(int64(1)); second != int64(42) {
		t.Errorf("the list holds %#v second, want 42", second)
	}

	single, err := call(func() string { return "one" })
	if err != nil || single != "one" {
		t.Errorf("a one-value return = (%#v, %v), want the value", single, err)
	}

	none, err := call(func() {})
	if err != nil || none != nil {
		t.Errorf("a void return = (%#v, %v), want nothing", none, err)
	}

	withNil, err := call(func() (string, error) { return "ok", nil })
	if err != nil || withNil != "ok" {
		t.Errorf("a nil trailing error = (%#v, %v), want the value", withNil, err)
	}

	failing := errors.New("no such table")
	if _, err := call(func() (string, error) { return "", failing }); !errors.Is(err, failing) {
		t.Errorf("a trailing error was reported as %v, want the error", err)
	}
}

// TestNameCallError covers filling in the PHP name of an error raised while
// building a call. invokeAny works from the Go signature alone and has no name
// to report; only the dispatch site holds the name a script typed, and
// nameCallError leaves a filled-in name alone.
func TestNameCallError(t *testing.T) {
	t.Parallel()
	if got := nameCallError(nil, "substr"); got != nil {
		t.Errorf("nameCallError(nil) = %v, want nil", got)
	}

	count := &ArgumentCountError{Want: 1, Got: 2}
	nameCallError(count, "strlen")
	if count.Name != "strlen" {
		t.Errorf("the arity error names %q, want %q", count.Name, "strlen")
	}

	named := &ArgumentCountError{Name: "substr", Want: 3, Got: 4}
	nameCallError(named, "strlen")
	if named.Name != "substr" {
		t.Errorf("a named arity error was renamed to %q", named.Name)
	}

	mismatch := &TypeError{Position: 1, Want: "int", Got: "string"}
	nameCallError(mismatch, "str_repeat")
	if mismatch.Name != "str_repeat" {
		t.Errorf("the type error names %q, want %q", mismatch.Name, "str_repeat")
	}

	other := errors.New("no such table")
	if got := nameCallError(other, "query"); !errors.Is(got, other) {
		t.Errorf("nameCallError() = %v on an unrelated error, want the error", got)
	}
}

// TestRecoveredError covers what a recovered panic reaches a script as. A
// callback that threw carries its own error across the Go frames between it and
// the host boundary and a script catches its own exception; anything else is a
// host panic, named after what panicked.
func TestRecoveredError(t *testing.T) {
	t.Parallel()
	thrown := NewRuntimeException("no such invoice", 0)
	if got := recoveredError(callbackPanic{err: thrown}, "usort"); got != error(thrown) {
		t.Errorf("a callback's own error reached the script as %v", got)
	}

	host := recoveredError("nil map", "Storage::open")
	var panicked *HostPanicError
	if !errors.As(host, &panicked) {
		t.Fatalf("a host panic reached the script as %T, want a HostPanicError", host)
	}
	if panicked.Callable != "Storage::open" {
		t.Errorf("the host panic names %q, want %q", panicked.Callable, "Storage::open")
	}
}

// TestWantsContext covers the check on whether the runner puts the runtime
// context ahead of a binding's arguments. Only a first parameter of
// context.Context counts and a binding taking a string first receives the
// arguments a script passed and nothing in front of them.
func TestWantsContext(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		fn   any
		want bool
	}{
		{"a context first", func(context.Context) {}, true},
		{"a context and more", func(context.Context, string) {}, true},
		{"a string first", func(string, context.Context) {}, false},
		{"no parameters", func() {}, false},
		{"not a func", "text", false},
	} {
		if got := wantsContext(reflect.TypeOf(tc.fn)); got != tc.want {
			t.Errorf("%s: wantsContext() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestAdapt covers wrapping a Go callable in the uniform shape the evaluation
// environment carries, which is the one shape the engine's call closures
// assert. The wrapper coerces arguments through reflection and a shim can keep
// a natural Go signature.
func TestAdapt(t *testing.T) {
	t.Parallel()
	fn := adapt(func(n int64, text string) string { return text + ":" + phpString(n) })
	value, err := fn(int64(7), "n")
	if err != nil {
		t.Fatalf("the adapted call failed: %v", err)
	}
	if got, want := value, "n:7"; got != want {
		t.Errorf("the adapted call = %#v, want %#v", got, want)
	}

	// The wrapper renders an int64 reaching a string parameter the way PHP does,
	// and the binding reads "7" and never Go's own conversion of the code point.
	coerced, err := fn(int64(9), int64(7))
	if err != nil {
		t.Fatalf("the coerced call failed: %v", err)
	}
	if got, want := coerced, "7:9"; got != want {
		t.Errorf("the coerced call = %#v, want %#v", got, want)
	}

	failing := adapt(func() (string, error) { return "", errors.New("no such table") })
	if _, err := failing(); err == nil {
		t.Error("an adapted callable returning an error reported none")
	}
}
