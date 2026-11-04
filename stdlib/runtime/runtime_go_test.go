package runtime_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// goRun executes src on a runtime carrying the default surface, the surface
// the import adds to: Run installs what the source opted into.
func goRun(t *testing.T, src string) (string, error) {
	t.Helper()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	program, err := rt.Load(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = rt.Run(program)
	return out.String(), err
}

// goOutput runs src and pins what it echoed, the one view of go's semantics a
// script has: a length, a capacity, and what a conversion returns.
func goOutput(t *testing.T, src, want string) {
	t.Helper()
	out, err := goRun(t, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestRegisterGo pins the opt-in: a runtime answers for none of go's names
// until a program imports the namespace, and that keeps them out of
// function_exists and the generated reference. After the import every name the
// tables carry resolves qualified, and none of them resolves bare: binding the
// short name is the parser's job, per file.
func TestRegisterGo(t *testing.T) {
	t.Parallel()
	rt := runner.New(&strings.Builder{}, runner.Options{})
	stdlib.Register(rt)
	if rt.FunctionExists(`Runtime\Go\make`) {
		t.Fatal("the go bindings are installed before anything imported them")
	}
	if !rt.UseNamespace(model.GoNamespace) {
		t.Fatal("the import installed nothing")
	}
	for _, name := range model.GoBuiltinNames() {
		if !rt.FunctionExists(`Runtime\Go\` + name) {
			t.Errorf(`Runtime\Go\%s does not resolve after the import`, name)
		}
		if rt.FunctionExists(name) {
			t.Errorf("%s resolves bare: the import registered a php name", name)
		}
	}
	// A go type is a class as well as a conversion: `new Go\int64(5)` allocates
	// through the constructor and class_exists reads the same table.
	for _, name := range model.GoTypeNames() {
		if _, ok := rt.LookupConstructor(`Runtime\Go\` + name); !ok {
			t.Errorf(`Runtime\Go\%s is in no class table`, name)
		}
	}
}

// TestGoBareNames covers what the import is for: the builtins read as go writes
// them, with no namespace in the source. A leading separator and the case of
// the namespace are php's to ignore, and php's own name wins where php has one.
func TestGoBareNames(t *testing.T) {
	t.Parallel()
	for _, use := range []string{"use Runtime\\Go;", "use \\runtime\\go;"} {
		goOutput(t, "<?php\n"+use+`
$s = append(make([]string, 0, 2), "a");
echo len($s), cap($s), $s[0], int8(300), "\n";
`, "12a44\n")
	}
	// A class is an element type too. php carries an object in a value of any
	// type, the element the slice holds and the one make zeroes it with.
	goOutput(t, `<?php
use Runtime\Go;

class User
{
	public $name = "ada";
}

$users = append(make([]User, 1), new User());
echo len($users), $users[1]->name, "\n";
`, "2ada\n")
	// `use function Ns\f;` imports the one name php says it does. The install
	// reads the namespace the symbol sits in and the binding resolves; the
	// rest of go's builtins stay unbound and a qualified call reaches them.
	goOutput(t, `<?php
use function Runtime\Go\make;

echo Runtime\Go\len(make("[]int", 2)), "\n";
`, "2\n")
	// The parser folds a conversion of a literal and the binding converts the
	// same value at run time. Both read one table and both answer the same.
	goOutput(t, `<?php
use Runtime\Go;

$n = 300;
echo int8(300) === int8($n) ? "agree" : "differ", "\n";
`, "agree\n")
	// min is php's, which reads an array, and go's reads values: a file that
	// imported the namespace still calls php's by its bare name.
	goOutput(t, `<?php
use Runtime\Go;

echo min([3, 1, 2]), Runtime\Go\min(3, 1, 2), "\n";
`, "11\n")
}

// TestGoSlices covers make, append, len and cap over a go slice: the length and
// the capacity are the ones make allocated, append fills within the capacity,
// and a foreach reads the slice back by index.
func TestGoSlices(t *testing.T) {
	t.Parallel()
	goOutput(t, `<?php
use Runtime\Go;

$s = Go\make("[]string", 0, 3);
echo Go\len($s), ",", Go\cap($s), "\n";
$s = Go\append($s, "a", "b");
echo Go\len($s), ",", Go\cap($s), "\n";
foreach ($s as $i => $v) { echo $i, "=", $v, " "; }
echo "\n";
foreach (Go\make("[]int64", 2) as $v) { echo $v, ";"; }
echo "\n";
foreach (Go\make("[][]int", 1) as $v) { echo Go\len($v), ";"; }
echo "\n";
`, "0,3\n2,3\n0=a 1=b \n0;0;\n0;\n")
}

// TestGoArrays covers the same builtins over php's own values: append writes
// through the array handle a script holds, a null stands for go's empty slice,
// and len is go's, which counts a string's bytes.
func TestGoArrays(t *testing.T) {
	t.Parallel()
	goOutput(t, `<?php
use Runtime\Go;

$a = [1, 2];
Go\append($a, 3);
echo count($a), ":", implode(",", $a), "\n";
$b = Go\append(null, "x");
echo count($b), ":", implode(",", $b), "\n";
echo Go\len("héllo"), ",", Go\len([1, 2, 3]), ",", Go\len(null), "\n";
`, "3:1,2,3\n1:x\n6,3,0\n")
}

// TestGoConversions covers a builtin type's conversion: php reduces the value
// and go's width truncates it, leaving int8(300) at 44. A type is a class too,
// a missing argument is the zero value, and float32 is the width that loses
// precision.
func TestGoConversions(t *testing.T) {
	t.Parallel()
	goOutput(t, `<?php
use Runtime\Go;

echo Go\int8(300), " ", Go\uint8(-1), " ", Go\int64("12"), " ", Go\string(5), " ", Go\int32(2147483648), "\n";
echo Go\float32(0.1) === 0.1 ? "same" : "narrowed", "\n";
echo Go\int64(), ",", (new Go\int64(5)) + 1, ",", Go\bool("0") ? "t" : "f", "\n";
var_dump(class_exists('Runtime\Go\int64'));
`, "44 255 12 5 -2147483648\nnarrowed\n0,6,f\nbool(true)\n")
}

// TestGoCollections covers copy, delete and clear: copy stops at the shorter of
// the two and converts into the element type, delete and clear read a php array
// as well as a go collection, and a cleared slice keeps its length.
func TestGoCollections(t *testing.T) {
	t.Parallel()
	goOutput(t, `<?php
use Runtime\Go;

$dst = Go\make("[]string", 2);
echo Go\copy($dst, ["a", "b", "c"]), ":";
foreach ($dst as $v) { echo $v; }
echo "\n";
$a = ["x" => 1, "y" => 2];
Go\delete($a, "x");
echo implode(",", array_keys($a)), "\n";
Go\clear($a);
echo count($a), "\n";
Go\clear($dst);
foreach ($dst as $v) { echo "[", $v, "]"; }
echo "\n";
`, "2:ab\ny\n0\n[][]\n")
}

// TestGoPrinting covers min, max, print, println and panic. go's print writes
// to the stream echo writes to, not to stderr, and a panic is the exception a
// php catch can see.
func TestGoPrinting(t *testing.T) {
	t.Parallel()
	goOutput(t, `<?php
use Runtime\Go;

echo Go\min(3, 1, 2), Go\max(3, 1, 2), "\n";
Go\println("a", 1);
Go\print("b", 2);
echo "\n";
try {
	Go\panic("boom");
} catch (RuntimeException $e) {
	echo "caught: ", $e->getMessage(), "\n";
}
`, "13\na 1\nb2\ncaught: panic: boom\n")
}

// TestGoRefusals pins what each binding refuses, and the message it names the
// supported shape in: a width php renders no value of, a value with no length,
// and a shape nothing appends to.
func TestGoRefusals(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, src, want string }{
		{"make width", `$x = Go\make("[]int8", 1);`, "element types are"},
		{"make type", `$x = Go\make("int", 1);`, "cannot allocate"},
		{"make length", `$x = Go\make("[]int", 3, 1);`, "make no []int"},
		{"append shape", `$x = Go\append("text", 1);`, "cannot append to a string"},
		{"len shape", `$x = Go\len(new stdClass());`, "has no length"},
		{"cap shape", `$x = Go\cap(1);`, "has no capacity"},
		{"copy shape", `$x = Go\copy(1, [1]);`, "cannot copy into"},
		{"delete shape", `Go\delete(1, "k");`, "cannot delete from"},
		{"clear shape", `Go\clear("text");`, "cannot clear"},
		{"min empty", `$x = Go\min();`, "at least one value"},
		{"convert arity", `$x = Go\int64(1, 2);`, "one value and no more"},
	} {
		_, err := goRun(t, "<?php\nuse Runtime\\Go;\n"+test.src)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s = %v, want %s", test.name, err, test.want)
		}
	}
}
