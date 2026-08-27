package shared_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/stdlib/shared"
)

// TestParseStr pins the decoder against php's own output: every want was read
// off `parse_str($in, $o); var_export($o);` under php 8, then flattened.
func TestParseStr(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "nesting, append and last wins",
			in:   "a[b]=1&a[c][]=2&a[c][]=3&plain=x&last=1&last=2",
			want: "a.b=1 a.c.0=2 a.c.1=3 last=2 plain=x",
		},
		{
			name: "mangling applies to the top-level name only",
			in:   "a.b=1&c d=2&e[f.g]=3",
			want: "a_b=1 c_d=2 e.f.g=3",
		},
		{
			name: "an unterminated first bracket is one flat key",
			in:   "a[b=1&a]b=2&c[=3",
			want: "a]b=2 a_b=1 c_=3",
		},
		{
			name: "an unterminated later bracket ends the path",
			in:   "a[b][c=1",
			want: "a.b=1",
		},
		{
			name: "trailing text after a closed bracket is discarded",
			in:   "a[b]c=1&x[b]]=2&y[]z=3",
			want: "a.b=1 x.b=2 y.0=3",
		},
		{
			name: "the first close bracket wins",
			in:   "a[[b]]=1",
			want: "a.[b=1",
		},
		{
			name: "a scalar is replaced by an array",
			in:   "a=1&a[b]=2",
			want: "a.b=2",
		},
		{
			name: "an array is replaced by a scalar",
			in:   "a[b]=1&a=2",
			want: "a=2",
		},
		{
			name: "a replaced sub-array starts over",
			in:   "x[a][b]=1&x[a]=2&x[a][c]=3",
			want: "x.a.c=3",
		},
		{
			name: "append follows the highest integer key",
			in:   "a[]=1&a[3]=x&a[]=y",
			want: "a.0=1 a.3=x a.4=y",
		},
		{
			name: "a negative key does not move the append index",
			in:   "a[-1]=x&a[]=y",
			want: "a.-1=x a.0=y",
		},
		{
			name: "each empty top-level bracket is a new element",
			in:   "a[][]=1&a[][]=2",
			want: "a.0.0=1 a.1.0=2",
		},
		{
			name: "string and integer keys mix in one array",
			in:   "x[b][]=1&x[b][]=2&x[b][c]=3",
			want: "x.b.0=1 x.b.1=2 x.b.c=3",
		},
		{
			name: "an empty variable name is dropped",
			in:   "=5&[]=6&[a]=7&a[]=",
			want: "a.0=",
		},
		{
			name: "decoding happens before the brackets are read",
			in:   "k%5Ba%20b%5D=1&v=%2B+%26",
			want: "k.a b=1 v=+ &",
		},
		{
			name: "a malformed escape stays literal",
			in:   "a=%zz&b=%2&c=%41",
			want: "a=%zz b=%2 c=A",
		},
		{
			name: "keys are canonical integers or strings",
			in:   "9=x&08=y&0x1=z",
			want: "08=y 0x1=z 9=x",
		},
		{
			name: "empty pairs are skipped and a bare name is empty",
			in:   "a[b]=1&&&c=2&=3&d",
			want: "a.b=1 c=2 d=",
		},
		{
			name: "a plus in a name decodes to a space, then mangles",
			in:   "a+b=1&c=%41%42",
			want: "a_b=1 c=AB",
		},
		{
			name: "keys are case sensitive",
			in:   "A[B]=1&a[b]=2",
			want: "A.B=1 a.b=2",
		},
		{
			name: "space inside a bracket survives",
			in:   "a[ b ]=1",
			want: "a. b =1",
		},
		{
			name: "an empty string decodes to nothing",
			in:   "",
			want: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := flatten(shared.ParseStr(test.in, shared.Limits{})); got != test.want {
				t.Errorf("ParseStr(%q)\n got %q\nwant %q", test.in, got, test.want)
			}
		})
	}
}

// A variable nested past the limit is dropped whole, not truncated. php warns
// and drops it too.
func TestParseStrNestingLimit(t *testing.T) {
	tests := []struct {
		depth int
		want  string
	}{
		{63, "kept"},
		{64, "kept"},
		{65, "dropped"},
		{200, "dropped"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.depth), func(t *testing.T) {
			raw := "a" + strings.Repeat("[x]", test.depth) + "=1"
			got := "kept"
			if shared.ParseStr(raw, shared.Limits{}).Len() == 0 {
				got = "dropped"
			}
			if got != test.want {
				t.Errorf("depth %d: %s, want %s", test.depth, got, test.want)
			}
		})
	}
}

// Pairs past the limit are dropped and earlier ones kept, as php does.
func TestParseStrVarLimit(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 1500; i++ {
		fmt.Fprintf(&b, "k%d=1&", i)
	}
	if got := shared.ParseStr(b.String(), shared.Limits{}).Len(); got != shared.DefaultMaxVars {
		t.Fatalf("kept %d variables, want %d", got, shared.DefaultMaxVars)
	}
	if got := shared.ParseStr(b.String(), shared.Limits{MaxVars: 10}).Len(); got != 10 {
		t.Fatalf("kept %d variables, want 10", got)
	}
	if got := shared.ParseStr(b.String(), shared.Limits{MaxVars: -1}).Len(); got != 1500 {
		t.Fatalf("kept %d variables, want 1500", got)
	}
}

// flatten renders a decoded array as sorted, space-separated `path=value`, so
// a test states its expectation on one line. Nested paths join with a dot; no
// test uses a dot inside a key.
func flatten(arr *model.Array) string {
	var entries []string
	var walk func(prefix string, a *model.Array)
	walk = func(prefix string, a *model.Array) {
		a.Range(func(key, value any) bool {
			path := fmt.Sprint(key)
			if prefix != "" {
				path = prefix + "." + path
			}
			if nested, ok := value.(*model.Array); ok {
				walk(path, nested)
				return true
			}
			entries = append(entries, path+"="+fmt.Sprint(value))
			return true
		})
	}
	walk("", arr)
	sort.Strings(entries)
	return strings.Join(entries, " ")
}
