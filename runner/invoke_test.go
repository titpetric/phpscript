package runner_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// runBinding registers fn under name, runs src, and returns what the script
// printed along with the run error.
func runBinding(t *testing.T, name string, fn any, src string) (string, error) {
	t.Helper()
	program, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	rt.RegisterFunc(name, fn)
	runErr := rt.Run(program)
	return out.String(), runErr
}

// Both dispatch paths coerce a string parameter the way PHP renders the value.
// invokeFast recognises func(string) string; func(string, string) string falls
// through to reflection, where Go's own conversion would render int64(65) as
// the code point "A".
func TestInvokeStringArgumentCoercion(t *testing.T) {
	tests := []struct {
		name string
		fn   any
		src  string
		want string
	}{
		{
			name: "fast path",
			fn:   func(s string) string { return "[" + s + "]" },
			src:  `<?php echo shout(65);`,
			want: "[65]",
		},
		{
			name: "reflect path",
			fn:   func(a, b string) string { return "[" + a + "|" + b + "]" },
			src:  `<?php echo shout(65, 66);`,
			want: "[65|66]",
		},
		{
			name: "reflect path, mixed types",
			fn:   func(a, b string) string { return "[" + a + "|" + b + "]" },
			src:  `<?php echo shout(1.5, true);`,
			want: "[1.5|1]",
		},
		{
			name: "strings pass through untouched",
			fn:   func(s string) string { return "[" + s + "]" },
			src:  `<?php echo shout("A");`,
			want: "[A]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := runBinding(t, "shout", test.fn, test.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != test.want {
				t.Fatalf("got %q, want %q", out, test.want)
			}
		})
	}
}

// A call passing more arguments than the binding declares is refused on both
// dispatch paths, as PHP refuses it for an internal function. reflect.Value
// .Call panics on the same call, so the check has to run before either path.
func TestInvokeTooManyArguments(t *testing.T) {
	tests := []struct {
		name string
		fn   any
		src  string
	}{
		{"fast path", func(v any) any { return v }, `<?php echo shout(1, 2);`},
		{"reflect path", func(a, b string) string { return a + b }, `<?php echo shout("a", "b", "c");`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runBinding(t, "shout", test.fn, test.src)
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if !strings.Contains(err.Error(), "expects at most") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// Fewer arguments than the binding declares stays legal: a Go binding spells
// PHP's optional parameters as extra ones, and they are zero-padded.
func TestInvokeTooFewArgumentsPads(t *testing.T) {
	out, err := runBinding(t, "shout", func(a, b string) string { return "[" + a + "|" + b + "]" }, `<?php echo shout("a");`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "[a|]" {
		t.Fatalf("got %q, want %q", out, "[a|]")
	}
}

// A Go method reached through PHP follows the same argument rules as a
// registered function: the surplus is refused rather than panicking inside
// reflect.Value.Call, and the message names the method a script typed.
type counter struct{}

func (counter) Add(a, b string) string { return a + b }

func TestInvokeMethodArgumentRules(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		want    string
		wantErr string
	}{
		{
			name: "coerces a string parameter as PHP renders it",
			src:  `<?php $c = new Counter; echo $c->add(65, 66);`,
			want: "6566",
		},
		{
			name: "pads an omitted trailing argument",
			src:  `<?php $c = new Counter; echo $c->add("a");`,
			want: "a",
		},
		{
			name:    "refuses a surplus argument",
			src:     `<?php $c = new Counter; echo $c->add("a", "b", "c");`,
			wantErr: "add() expects at most 2 arguments, 3 given",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := parser.Parse(test.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var out strings.Builder
			rt := runner.New(&out, runner.Options{})
			rt.RegisterConstructor("Counter", func() counter { return counter{} })
			runErr := rt.Run(program)
			if test.wantErr != "" {
				if runErr == nil || !strings.Contains(runErr.Error(), test.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", runErr, test.wantErr)
				}
				return
			}
			if runErr != nil {
				t.Fatalf("run: %v", runErr)
			}
			if out.String() != test.want {
				t.Fatalf("got %q, want %q", out.String(), test.want)
			}
		})
	}
}

// A string passed to a Go API that declares time.Duration uses Go's duration
// syntax. The rule lives at the shared reflection boundary, so automatically
// exposed methods such as time.Time.Add need no PHP-specific adapter.
func TestInvokeMethodParsesDurationString(t *testing.T) {
	program, err := parser.Parse(`<?php $t = new ClockTime; $later = $t->add("30m"); echo $later->format("15:04");`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	rt.RegisterConstructor("ClockTime", func() time.Time {
		return time.Date(2026, time.August, 26, 14, 48, 0, 0, time.UTC)
	})
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != "15:18" {
		t.Fatalf("got %q, want %q", out.String(), "15:18")
	}
}

func TestInvokeBindingParsesDurationString(t *testing.T) {
	out, err := runBinding(t, "millis", func(duration time.Duration) int64 {
		return duration.Milliseconds()
	}, `<?php echo millis("250ms");`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "250" {
		t.Fatalf("got %q, want %q", out, "250")
	}
}

// Every throwable a catch clause can bind answers the Throwable method set,
// whether it came from a thrown Exception, a binding that returned an error,
// or a panic converted at the host boundary.
func TestThrowableMethods(t *testing.T) {
	tests := []struct {
		name string
		fn   any
		src  string
		want string
	}{
		{
			name: "error returned by a binding",
			fn:   func(v any) (any, error) { return nil, errProbe },
			src:  `<?php try { boom("x"); } catch (Throwable $e) { echo $e->getMessage(); }`,
			want: "probe failed",
		},
		{
			name: "panic converted at the host boundary",
			fn:   func(v any) any { panic("exploded") },
			src:  `<?php try { boom("x"); } catch (Throwable $e) { echo $e->getMessage(); }`,
			want: "host panic in func(interface {}) interface {}: exploded",
		},
		{
			name: "code defaults to zero",
			fn:   func(v any) (any, error) { return nil, errProbe },
			src:  `<?php try { boom("x"); } catch (Throwable $e) { echo $e->getCode(); }`,
			want: "0",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := runBinding(t, "boom", test.fn, test.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != test.want {
				t.Fatalf("got %q, want %q", out, test.want)
			}
		})
	}
}

var errProbe = probeError("probe failed")

type probeError string

func (e probeError) Error() string { return string(e) }

// A thrown Exception reports the code it was constructed with, rather than the
// zero the Throwable fallback supplies for a bare Go error. This needs the
// standard library, which is where the Exception class is registered.
func TestThrownExceptionKeepsItsCode(t *testing.T) {
	program, err := parser.Parse(`<?php try { throw new Exception("m", 7); } catch (Throwable $e) { echo $e->getMessage() . ":" . $e->getCode(); }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != "m:7" {
		t.Fatalf("got %q, want %q", out.String(), "m:7")
	}
}

// An argument that converts to no declared parameter type is refused before
// reflect.Value.Call sees it, as a PHP TypeError naming the function, the
// position and the types involved, rather than a host panic a script cannot
// distinguish from a bug in the binding.
func TestInvokeUnconvertibleArgument(t *testing.T) {
	tests := []struct {
		name string
		fn   any
		src  string
		want string
	}{
		{
			name: "array for an int parameter",
			fn:   func(s string, limit int64) string { return s },
			src:  `<?php try { shout("a", array()); } catch (Throwable $e) { echo $e->getMessage(); }`,
			want: "shout(): Argument #2 must be of type int, array given",
		},
		{
			name: "int for a callable parameter",
			fn:   func(s string, out ...func(any)) string { return s },
			src:  `<?php try { shout("a", 1); } catch (Throwable $e) { echo $e->getMessage(); }`,
			want: "shout(): Argument #2 must be of type callable, int given",
		},
		{
			// The injected runtime context sits ahead of the script's
			// arguments and does not count towards the reported position.
			name: "position skips an injected context",
			fn:   func(ctx context.Context, s string, limit int64) string { return s },
			src:  `<?php try { shout("a", array()); } catch (Throwable $e) { echo $e->getMessage(); }`,
			want: "shout(): Argument #2 must be of type int, array given",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := runBinding(t, "shout", test.fn, test.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != test.want {
				t.Fatalf("got %q, want %q", out, test.want)
			}
		})
	}
}

// Calling a function no table knows reports the condition the way the flatstack
// engine reports it, rather than the VM's "cannot call nil".
func TestUndefinedFunctionMessage(t *testing.T) {
	program, err := parser.Parse(`<?php nope();`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	runErr := runner.New(&out, runner.Options{}).Run(program)
	if runErr == nil {
		t.Fatal("want an error, got nil")
	}
	if !strings.Contains(runErr.Error(), "call to undefined function nope()") {
		t.Fatalf("err = %v", runErr)
	}
}

// The stub installed for an unknown name must not outlive the declaration of a
// function by that name. Registering one moves the function-table generation,
// which is what makes the evaluation environment (and the stub in it) rebuild.
func TestUndefinedFunctionStubIsNotSticky(t *testing.T) {
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})

	// The call site is compiled and evaluated while the name is unknown, so the
	// environment holds the stub before the declaration below runs.
	miss, err := parser.Parse(`<?php try { greet(); } catch (Throwable $e) { echo "undefined\n"; }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(miss); err != nil {
		t.Fatalf("run: %v", err)
	}

	declared, err := parser.Parse("<?php function greet() { return \"hi\"; } echo greet(), \"\\n\";")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(declared); err != nil {
		t.Fatalf("run: %v", err)
	}

	// A host binding registered after the miss resolves for the same reason.
	rt.RegisterFunc("shout", func(s string) string { return s })
	shout, err := parser.Parse(`<?php echo shout("ho"), "\n";`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(shout); err != nil {
		t.Fatalf("run: %v", err)
	}

	if got, want := out.String(), "undefined\nhi\nho\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
