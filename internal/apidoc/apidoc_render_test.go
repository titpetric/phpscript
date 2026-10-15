package apidoc

import (
	"strings"
	"testing"
)

// TestFunc_Signature covers the two forms a registered function is cited in.
// A plain name is declarable, so it is written with the function keyword. A
// Class::method name is not, so it is written the way a script calls it, and
// pasting the line into a .php file would be a parse error either way round.
func TestFunc_Signature(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		fn   Func
		want string
	}{
		{
			name: "no parameters",
			fn:   Func{Name: "microtime", Returns: "float"},
			want: "function microtime(): float",
		},
		{
			name: "one parameter",
			fn: Func{Name: "strlen", Returns: "int", Params: []Param{
				{Name: "string", Type: "string"},
			}},
			want: "function strlen(string $string): int",
		},
		{
			name: "variadic and by-reference",
			fn: Func{Name: "preg_match", Returns: "int", Params: []Param{
				{Name: "pattern", Type: "string"},
				{Name: "subject", Type: "string"},
				{Name: "matches", ByRef: true},
			}},
			want: "function preg_match(string $pattern, string $subject, &$matches): int",
		},
		{
			name: "static method is cited as a call",
			fn: Func{Name: "Database::register", Returns: "bool", Params: []Param{
				{Name: "name", Type: "string"},
				{Name: "dsn", Type: "string"},
			}},
			want: "Database::register(string $name, string $dsn): bool",
		},
		{
			name: "void return",
			fn:   Func{Name: "flush", Returns: "void"},
			want: "function flush(): void",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn.Signature(); got != tt.want {
				t.Errorf("Signature() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestClass_Signature covers the constructor line of a class stub. It carries
// no return type: PHP's __construct declares none, and the class it builds is
// the heading the stub sits under.
func TestClass_Signature(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		class Class
		want  string
	}{
		{
			name:  "no parameters",
			class: Class{Name: "Session"},
			want:  "public function __construct()",
		},
		{
			name: "one parameter",
			class: Class{Name: "Database", Params: []Param{
				{Name: "name", Type: "string"},
			}},
			want: "public function __construct(string $name)",
		},
		{
			name: "variadic",
			class: Class{Name: `Database\Migrate`, Params: []Param{
				{Name: "args", Type: "string", Variadic: true},
			}},
			want: "public function __construct(string ...$args)",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.class.Signature(); got != tt.want {
				t.Errorf("Signature() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMethod_Signature covers a method line of a class stub. The name is the
// PHP spelling in snake_case, because method dispatch is case-insensitive with
// underscores accepted and the stub documents the spelling a script types.
func TestMethod_Signature(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		method Method
		want   string
	}{
		{
			name:   "no parameters",
			method: Method{Name: "insert_id", Returns: "int"},
			want:   "public function insert_id(): int",
		},
		{
			name: "two parameters",
			method: Method{Name: "get_all", Returns: "array", Params: []Param{
				{Name: "query", Type: "string"},
				{Name: "args", Type: "mixed", Variadic: true},
			}},
			want: "public function get_all(string $query, mixed ...$args): array",
		},
		{
			name:   "void return",
			method: Method{Name: "close", Returns: "void"},
			want:   "public function close(): void",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.method.Signature(); got != tt.want {
				t.Errorf("Signature() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderParam(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   Param
		want string
	}{
		{Param{Name: "subject", Type: "string"}, "string $subject"},
		{Param{Name: "args", Type: "mixed", Variadic: true}, "mixed ...$args"},
		{Param{Name: "matches", ByRef: true}, "&$matches"},
		{Param{Name: "matches", ByRef: true, Variadic: true}, "&$matches = null"},
	}
	for _, c := range cases {
		if got := renderParam(c.in); got != c.want {
			t.Errorf("renderParam(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRenderWritesOneStubPerRegistration covers the shape of the generated
// page: functions grouped under the Go package that registered them, classes
// as PHP class stubs, a one-line doc comment as a line comment and a longer
// one as a docblock. The aliases line names the extra class names one
// constructor's registered name.
func TestRenderWritesOneStubPerRegistration(t *testing.T) {
	t.Parallel()
	funcs := []Func{
		{Name: "strlen", Package: "stdlib/core", Area: "strings", Returns: "int",
			Comment: []string{"strlen returns the byte length of $string."},
			Params:  []Param{{Name: "string", Type: "string"}}},
		{Name: "json_encode", Package: "stdlib/core", Area: "json", Returns: "string",
			Params: []Param{{Name: "value", Type: "mixed"}}},
	}
	classes := []Class{{
		Name:    "RuntimeException",
		Aliases: []string{"LogicException"},
		Package: "stdlib",
		Comment: []string{"RuntimeException is thrown at run time.", "It carries a message and a code."},
		Params:  []Param{{Name: "message", Type: "string"}},
		Methods: []Method{{Name: "get_message", Returns: "string"}},
	}}

	page := render(funcs, classes)

	for _, want := range []string{
		"### stdlib/core",
		"#### json",
		"#### strings",
		"// strlen returns the byte length of $string.",
		"function strlen(string $string): int",
		"function json_encode(mixed $value): string",
		"### `RuntimeException`",
		"Registered from `stdlib`.",
		"/**\n * RuntimeException is thrown at run time.\n * It carries a message and a code.\n */",
		"class RuntimeException\n{\n    public function __construct(string $message) {}",
		"    public function get_message(): string {}",
		"Also registered with this constructor: `LogicException`.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("rendered page does not contain %q:\n%s", want, page)
		}
	}

	// The page ends on exactly one newline. The file is committed, so a run
	// that found no change has to write the same bytes.
	if !strings.HasSuffix(page, "\n") || strings.HasSuffix(page, "\n\n") {
		t.Errorf("rendered page ends with %q, want one newline", page[max(0, len(page)-8):])
	}
}
