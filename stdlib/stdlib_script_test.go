package stdlib_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// runScript parses and runs src against a runtime with the stdlib installed,
// returning its output. The shims are exercised through the VM because that is
// where their return shape has to hold up: foreach, indexing, count() and
// property writes all reach the value through reflection.
func runScript(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// TestExceptionSemantics covers what returning *Exception instead of a struct
// value buys: the instance the script holds is addressable, so its fields are
// writable and a write is visible through the accessors.
func TestExceptionSemantics(t *testing.T) {
	cases := []struct {
		name string
		php  string
		want string
	}{
		{
			name: "accessors",
			php:  `<?php $e = new Exception("boom", 7); echo $e->getMessage() . ":" . $e->getCode();`,
			want: "boom:7",
		},
		{
			name: "default code",
			php:  `<?php $e = new Exception("boom"); echo $e->getMessage() . ":" . $e->getCode();`,
			want: "boom:0",
		},
		{
			name: "field write is visible to the accessor",
			php:  `<?php $e = new Exception("boom", 7); $e->message = "changed"; echo $e->getMessage();`,
			want: "changed",
		},
		{
			name: "code field write",
			php:  `<?php $e = new Exception("boom", 7); $e->code = 404; echo $e->getCode();`,
			want: "404",
		},
		{
			name: "echo renders the message through the error interface",
			php:  `<?php $e = new Exception("boom", 7); echo $e;`,
			want: "boom",
		},
		{
			name: "reference semantics across a call",
			php: `<?php
function rename($e) { $e->message = "renamed"; }
$e = new Exception("boom");
rename($e);
echo $e->getMessage();`,
			want: "renamed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runScript(t, tc.php); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestArrayMergeScript pins array_merge's behaviour through the VM: the list
// fast path must be indistinguishable from the *model.Array it replaced for
// everything except appending.
func TestArrayMergeScript(t *testing.T) {
	cases := []struct {
		name string
		php  string
		want string
	}{
		{
			name: "lists are renumbered and concatenated",
			php:  `<?php $a = array_merge(array("a", "b"), array("c")); echo implode(",", $a) . "|" . count($a) . "|" . $a[2];`,
			want: "a,b,c|3|c",
		},
		{
			name: "foreach over a merged list",
			php:  `<?php foreach (array_merge(array("a"), array("b", "c")) as $k => $v) { echo $k . ":" . $v . " "; }`,
			want: "0:a 1:b 2:c ",
		},
		{
			name: "later string keys win",
			php:  `<?php $m = array_merge(array("a" => 1, "b" => 2), array("a" => 3)); echo $m["a"] . "," . $m["b"] . "," . count($m);`,
			want: "3,2,2",
		},
		{
			name: "string and int keys mix",
			php:  `<?php $m = array_merge(array("a" => 1), array("x", "y")); echo $m["a"] . "," . $m[0] . "," . $m[1];`,
			want: "1,x,y",
		},
		{
			name: "string-keyed merge stays appendable",
			php:  `<?php $m = array_merge(array("a" => 1), array("x")); $m[] = "z"; echo count($m) . "," . $m[1];`,
			want: "3,z",
		},
		{
			name: "merged list forwards to call_user_func_array",
			php: `<?php
$join3 = function ($a, $b, $c) { return $a . $b . $c; };
echo call_user_func_array($join3, array_merge(array("x"), array("y", "z")));`,
			want: "xyz",
		},
		{
			name: "empty merge",
			php:  `<?php $a = array_merge(); echo count($a) . (empty($a) ? ",empty" : ",set");`,
			want: "0,empty",
		},
		{
			name: "is_array holds for the merged list",
			php:  `<?php echo is_array(array_merge(array(1), array(2))) ? "yes" : "no";`,
			want: "yes",
		},
		{
			name: "json_encode of a merged list is a JSON array",
			php:  `<?php echo json_encode(array_merge(array("a"), array("b")));`,
			want: `["a","b"]`,
		},
		{
			name: "merged list element write",
			php:  `<?php $a = array_merge(array("a"), array("b")); $a[0] = "z"; echo implode(",", $a);`,
			want: "z,b",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runScript(t, tc.php); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStringShimsScript(t *testing.T) {
	cases := []struct {
		name string
		php  string
		want string
	}{
		{
			name: "htmlspecialchars escapes the five entities",
			php:  `<?php echo htmlspecialchars("<a href=\"x\">a & b's</a>");`,
			want: `&lt;a href=&quot;x&quot;&gt;a &amp; b&#039;s&lt;/a&gt;`,
		},
		{
			name: "htmlspecialchars passes plain text through",
			php:  `<?php echo htmlspecialchars("plain text");`,
			want: "plain text",
		},
		{
			name: "crc32",
			php:  `<?php echo crc32("The quick brown fox jumped over the lazy dog.");`,
			want: "2191738434",
		},
		{
			name: "crc32 empty",
			php:  `<?php echo crc32("");`,
			want: "0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runScript(t, tc.php); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// BenchmarkScriptHTMLSpecialChars measures the shim the way a template does:
// through the VM's reflection call path.
func BenchmarkScriptHTMLSpecialChars(b *testing.B) {
	prog, err := parser.Parse(`<?php echo htmlspecialchars("<a href=\"/x?a=1&b=2\">Tom & Jerry's</a>");`)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out.Reset()
		if err := rt.Run(prog); err != nil {
			b.Fatalf("run: %v", err)
		}
	}
}

// BenchmarkScriptArrayMerge measures the call_user_func_array shape end to end.
func BenchmarkScriptArrayMerge(b *testing.B) {
	prog, err := parser.Parse(`<?php $a = array_merge(array("SELECT 1"), array("a", "b", "c", "d"));`)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out.Reset()
		if err := rt.Run(prog); err != nil {
			b.Fatalf("run: %v", err)
		}
	}
}
