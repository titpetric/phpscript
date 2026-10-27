package apidoc

import (
	"context"
	"reflect"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// reflectedStruct is a Go struct reached as a parameter or a result.
type reflectedStruct struct {
	Name string
}

// reflectedInt is a named integer type, as time.Duration is: PHP sees the
// kind and never the name.
type reflectedInt int64

// reflectedCloser is an interface with a method, which PHP sees as an object.
type reflectedCloser interface {
	Close() error
}

// TestPHPTypeReflect covers the Go kind to PHP type map. A []byte is a string
// because that is what a script holding binary data has; every other slice,
// array and map is an array; a pointer reads through to what it points at; and
// an empty interface maps to mixed, one with a method to an object.
func TestPHPTypeReflect(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		value any
		want  string
	}{
		{value: "", want: "string"},
		{value: true, want: "bool"},
		{value: int(0), want: "int"},
		{value: int8(0), want: "int"},
		{value: int64(0), want: "int"},
		{value: uint64(0), want: "int"},
		{value: reflectedInt(0), want: "int"},
		{value: float32(0), want: "float"},
		{value: float64(0), want: "float"},
		{value: []byte(nil), want: "string"},
		{value: []string(nil), want: "array"},
		{value: []any(nil), want: "array"},
		{value: [2]int{}, want: "array"},
		{value: map[string]any(nil), want: "array"},
		{value: reflectedStruct{}, want: "object"},
		{value: &reflectedStruct{}, want: "object"},
		{value: model.NewArraySize(0), want: "array"},
		{value: func(string) bool { return false }, want: "callable"},
	} {
		got := phpTypeReflect(reflect.TypeOf(tt.value))
		if got != tt.want {
			t.Errorf("phpTypeReflect(%T) = %q, want %q", tt.value, got, tt.want)
		}
	}

	// An interface has no value to reflect from and the two cases spell types.
	if got := phpTypeReflect(reflect.TypeOf((*any)(nil)).Elem()); got != "mixed" {
		t.Errorf("phpTypeReflect(any) = %q, want mixed", got)
	}
	if got := phpTypeReflect(reflect.TypeOf((*reflectedCloser)(nil)).Elem()); got != "object" {
		t.Errorf("phpTypeReflect(an interface with a method) = %q, want object", got)
	}
}

// TestIsRefSetterType covers the shape the runner passes for a by-reference
// parameter: one empty-interface argument and no result. preg_match's
// $matches arrives that way and the signature carries an ampersand and no
// type.
func TestIsRefSetterType(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		value any
		want  bool
	}{
		{name: "the setter", value: func(any) {}, want: true},
		{name: "returns something", value: func(any) error { return nil }, want: false},
		{name: "concrete argument", value: func(string) {}, want: false},
		{name: "variadic", value: func(...any) {}, want: false},
		{name: "two arguments", value: func(any, any) {}, want: false},
		{name: "no argument", value: func() {}, want: false},
		{name: "an interface with a method", value: func(reflectedCloser) {}, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRefSetterType(reflect.TypeOf(tt.value)); got != tt.want {
				t.Errorf("isRefSetterType(%T) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}

// TestReflectParams covers the parameter list a binding documents when there
// is no Go source to read names from. reflectParams drops a context argument,
// takes the remaining names from the PHP type, and numbers a repeated name so
// two ignored arguments do not both print as $value.
func TestReflectParams(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		value any
		want  []string
	}{
		{name: "none", value: func() {}, want: nil},
		{name: "one string", value: func(string) {}, want: []string{"string $string"}},
		{name: "context is dropped", value: func(context.Context, string) {}, want: []string{"string $string"}},
		{
			name:  "repeated names are numbered",
			value: func(string, string) {},
			want:  []string{"string $string1", "string $string2"},
		},
		{
			name:  "one name per php type",
			value: func(int64, float64, bool, []string, func(string) bool) {},
			want: []string{
				"int $num1", "float $num2", "bool $flag",
				"array $array", "callable $callback",
			},
		},
		{
			name:  "variadic",
			value: func(string, ...any) {},
			want:  []string{"string $string", "mixed ...$args"},
		},
		{
			name:  "by reference",
			value: func(string, func(any)) {},
			want:  []string{"string $string", "&$ref"},
		},
		{
			name:  "variadic by reference",
			value: func(string, ...func(any)) {},
			want:  []string{"string $string", "&$ref = null"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			params := reflectParams(reflect.TypeOf(tt.value))
			if len(params) != len(tt.want) {
				t.Fatalf("reflectParams(%T) = %s, want %v", tt.value, paramList(params), tt.want)
			}
			for i, want := range tt.want {
				if got := renderParam(params[i]); got != want {
					t.Errorf("parameter %d = %q, want %q", i, got, want)
				}
			}
		})
	}
}

// TestReflectReturn covers the one PHP type a Go result list folds into. A
// trailing error reaches the script as a throw and never as a return and a
// function returning only an error is void. Two or more non-error results are
// one array: the runtime packs them into a list for the script to destructure.
func TestReflectReturn(t *testing.T) {
	t.Parallel()
	classTypes := map[reflect.Type]string{
		reflect.TypeOf(reflectedStruct{}): "Thing",
		reflect.TypeOf(reflectedInt(0)):   `Thing\Span`,
	}

	for _, tt := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "nothing", value: func() {}, want: "void"},
		{name: "only an error", value: func() error { return nil }, want: "void"},
		{name: "one value", value: func() string { return "" }, want: "string"},
		{name: "a value and an error", value: func() (string, error) { return "", nil }, want: "string"},
		// Two values a script can hold reach it as a list and the declared
		// return is the array and not the pair of types in it.
		{name: "two values", value: func() (int64, bool) { return 0, false }, want: "array"},
		{name: "a registered struct", value: func() reflectedStruct { return reflectedStruct{} }, want: "Thing"},
		{name: "a registered named int", value: func() reflectedInt { return 0 }, want: `Thing\Span`},
		{
			name:  "a registered type and an error",
			value: func() (reflectedInt, error) { return 0, nil },
			want:  `Thing\Span`,
		},
		{
			name:  "an unregistered pointer",
			value: func() *reflectedStruct { return nil },
			want:  "object",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := reflectReturn(reflect.TypeOf(tt.value), classTypes)
			if got != tt.want {
				t.Errorf("reflectReturn(%T) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestHasRegisteredReturn covers the test that decides whether a Go function's
// result is a class a script can hold. A registered type is one; the error
// beside it never counts, and neither does a plain string.
func TestHasRegisteredReturn(t *testing.T) {
	t.Parallel()
	classTypes := map[reflect.Type]string{reflect.TypeOf(reflectedStruct{}): "Thing"}

	for _, tt := range []struct {
		name  string
		value any
		want  bool
	}{
		{name: "registered", value: func() reflectedStruct { return reflectedStruct{} }, want: true},
		{name: "registered with an error", value: func() (reflectedStruct, error) { return reflectedStruct{}, nil }, want: true},
		{name: "a pointer to it is a different type", value: func() *reflectedStruct { return nil }},
		{name: "a string", value: func() string { return "" }},
		{name: "nothing", value: func() {}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasRegisteredReturn(reflect.TypeOf(tt.value), classTypes); got != tt.want {
				t.Errorf("hasRegisteredReturn(%T) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}
