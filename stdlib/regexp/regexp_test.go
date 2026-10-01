package regexp_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// run executes src on both engines and returns what the interpreter printed,
// failing when the two disagree. The binding is reached through reflection in
// either case, so a divergence would be the bytecode engine taking a different
// path to the same registration rather than a different regexp.
func run(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var interpreted, flat strings.Builder
	for _, engine := range []struct {
		name string
		out  *strings.Builder
		rt   *runner.Runtime
	}{
		{name: "runtime", out: &interpreted, rt: runner.New(&interpreted, runner.Options{})},
		{name: "flatstack", out: &flat, rt: flatstack.New(&flat, flatstack.Options{})},
	} {
		stdlib.Register(engine.rt)
		if err := engine.rt.Run(prog); err != nil {
			t.Fatalf("%s: run: %v (output %q)", engine.name, err, engine.out.String())
		}
	}
	if interpreted.String() != flat.String() {
		t.Fatalf("engines disagree: runtime %q, flatstack %q", interpreted.String(), flat.String())
	}
	return interpreted.String()
}

func TestRegexpCompile(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "find_string",
			src:  `echo (new Regexp\Compile('\d+'))->find_string("abc 42 def");`,
			want: "42",
		},
		{
			name: "match_string",
			src:  `echo (new Regexp\Compile('^a.c$'))->match_string("abc") ? "yes" : "no";`,
			want: "yes",
		},
		{
			name: "find_all_string_submatch",
			src: `$m = (new Regexp\Compile('(\w)(\d)'))->find_all_string_submatch("a1 b2", -1);
				echo $m[0][1], $m[0][2], $m[1][1], $m[1][2];`,
			want: "a1b2",
		},
		{
			// A []byte return is the text it carries, not a list of integers.
			name: "find returns bytes as a string",
			src: `$b = (new Regexp\Compile('\d+'))->find("abc 42 def");
				echo $b, ":", strlen($b), ":", gettype($b);`,
			want: "42:2:string",
		},
		{
			name: "find_all_submatch nests",
			src: `foreach ((new Regexp\Compile('(\w)(\d)'))->find_all_submatch("a1 b2", -1) as $m) {
					echo $m[1], $m[2];
				}`,
			want: "a1b2",
		},
		{
			// Two non-error Go results arrive as a PHP list.
			name: "literal_prefix returns both results",
			src: `list($prefix, $complete) = (new Regexp\Compile('abc'))->literal_prefix();
				echo $prefix, ":", $complete ? "complete" : "partial";`,
			want: "abc:complete",
		},
		{
			name: "class name is the Go type name",
			src:  `echo get_class(new Regexp\Compile('.'));`,
			want: "Regexp",
		},
		{
			// POSIX takes the leftmost-longest match, Perl syntax the first.
			name: "compile_posix is leftmost-longest",
			src: `echo (new Regexp\Compile('a|ab'))->find_string("xabz"), ":",
					(new Regexp\CompilePOSIX('a|ab'))->find_string("xabz");`,
			want: "a:ab",
		},
		{
			name: "a closure fills a Go callback parameter",
			src: `echo (new Regexp\Compile('\d'))->replace_all_string_func("a1b2", function ($d) {
					return "[" . $d . "]";
				});`,
			want: "a[1]b[2]",
		},
		{
			name: "a pattern that does not parse throws",
			src: `try {
					new Regexp\Compile('(');
					echo "compiled";
				} catch (Exception $e) {
					echo $e->getMessage();
				}`,
			want: "error parsing regexp: missing closing ): `(`",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := run(t, "<?php "+test.src); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}
