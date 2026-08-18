package tests_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/tests"
)

// newBindingRuntime builds a runtime with the stdlib and the example bindings
// from bindings.go installed.
func newBindingRuntime(out *strings.Builder) *runner.Runtime {
	rt := runner.New(out, runner.Options{})
	stdlib.Register(rt, tests.RegisterBindings)
	return rt
}

// runBinding parses and runs src, returning its output.
func runBinding(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := newBindingRuntime(&out)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// TestBindingReturnShapes is the core claim under test: a binding may return a
// native Go slice, map, struct or pointer behind `any`, and the VM consumes it
// through reflection exactly like a *model.Array.
func TestBindingReturnShapes(t *testing.T) {
	cases := []struct {
		name string
		php  string
		want string
	}{
		// foreach over each list shape yields the same values in the same order.
		{
			name: "foreach over model.Array",
			php:  `<?php foreach (bind_list_array() as $k => $v) { echo $k . ":" . $v . " "; }`,
			want: "0:alpha 1:beta 2:gamma 3:delta 4:epsilon ",
		},
		{
			name: "foreach over []string",
			php:  `<?php foreach (bind_list_strings() as $k => $v) { echo $k . ":" . $v . " "; }`,
			want: "0:alpha 1:beta 2:gamma 3:delta 4:epsilon ",
		},
		{
			name: "foreach over []any",
			php:  `<?php foreach (bind_list_any() as $k => $v) { echo $k . ":" . $v . " "; }`,
			want: "0:alpha 1:beta 2:gamma 3:delta 4:epsilon ",
		},
		{
			name: "foreach over slice returned behind any",
			php:  `<?php foreach (bind_any_strings() as $k => $v) { echo $k . ":" . $v . " "; }`,
			want: "0:alpha 1:beta 2:gamma 3:delta 4:epsilon ",
		},
		{
			name: "foreach over zero-copy slice",
			php:  `<?php foreach (bind_list_shared() as $v) { echo $v . " "; }`,
			want: "alpha beta gamma delta epsilon ",
		},

		// Integer indexing works on slices via helperIndex's reflect fallback.
		{
			name: "index []string",
			php:  `<?php $l = bind_list_strings(); echo $l[0] . "," . $l[4];`,
			want: "alpha,epsilon",
		},
		{
			name: "index model.Array",
			php:  `<?php $l = bind_list_array(); echo $l[0] . "," . $l[4];`,
			want: "alpha,epsilon",
		},
		{
			name: "out of range slice index is null",
			php:  `<?php $l = bind_list_strings(); echo "[" . $l[99] . "]";`,
			want: "[]",
		},

		// String keys work on Go maps.
		{
			name: "index map[string]any",
			php:  `<?php $m = bind_map(); echo $m["name"] . "/" . $m["id"];`,
			want: "alpha/1",
		},
		{
			name: "nested rows as []map[string]any",
			php:  `<?php foreach (bind_rows_maps() as $row) { echo $row["id"] . "=" . $row["name"] . " "; }`,
			want: "0=alpha 1=beta 2=gamma 3=delta 4=epsilon ",
		},
		{
			name: "nested rows as model.Array",
			php:  `<?php foreach (bind_rows_array() as $row) { echo $row["id"] . "=" . $row["name"] . " "; }`,
			want: "0=alpha 1=beta 2=gamma 3=delta 4=epsilon ",
		},

		// Structs behave as objects: exported fields are readable
		// case-insensitively, exported methods are callable.
		{
			name: "struct pointer property access",
			php:  `<?php $r = bind_record(); echo $r->name . "#" . $r->id;`,
			want: "alpha#1",
		},
		{
			name: "struct property access matches Go casing too",
			php:  `<?php $r = bind_record(); echo $r->Name;`,
			want: "alpha",
		},
		{
			name: "struct value property access",
			php:  `<?php $r = bind_record_value(); echo $r->name;`,
			want: "alpha",
		},
		{
			name: "struct method dispatch",
			php:  `<?php $r = bind_record(); echo $r->label();`,
			want: "alpha",
		},
		{
			name: "slice of structs",
			php:  `<?php foreach (bind_records() as $r) { echo $r->name . " "; }`,
			want: "alpha beta gamma delta epsilon ",
		},
		{
			name: "model.Object property access",
			php:  `<?php $o = bind_object(); echo $o->name . "#" . $o->id;`,
			want: "alpha#1",
		},

		// Scalars round-trip unchanged.
		{
			name: "int64 return",
			php:  `<?php echo bind_int() + 1;`,
			want: "4097",
		},
		{
			name: "int64 behind any",
			php:  `<?php echo bind_int_any() + 1;`,
			want: "4097",
		},
		{
			name: "bool return",
			php:  `<?php if (bind_bool()) { echo "yes"; }`,
			want: "yes",
		},

		// A trailing error surfaces to PHP as a catchable throw.
		{
			name: "error return is catchable",
			php:  `<?php try { bind_split("a,b", ""); } catch (Exception $e) { echo "caught:" . $e; }`,
			want: "caught:bind_split: empty separator",
		},
		{
			name: "successful multi-return yields the value",
			php:  `<?php foreach (bind_split("a,b,c", ",") as $v) { echo $v; }`,
			want: "abc",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runBinding(t, tc.php); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBindingStdlibInteropAcrossShapes pins the behaviour of the stdlib
// predicates and collection helpers against native Go return shapes. These are
// the functions a script reaches for after calling a binding, and they are the
// blast radius of switching a binding away from *model.Array.
func TestBindingStdlibInteropAcrossShapes(t *testing.T) {
	cases := []struct {
		name string
		php  string
		want string
	}{
		{
			name: "count model.Array",
			php:  `<?php echo count(bind_list_array());`,
			want: "5",
		},
		{
			name: "count []string",
			php:  `<?php echo count(bind_list_strings());`,
			want: "5",
		},
		{
			name: "count []map",
			php:  `<?php echo count(bind_rows_maps());`,
			want: "5",
		},
		{
			name: "count map",
			php:  `<?php echo count(bind_map());`,
			want: "2",
		},
		{
			name: "count scalar is zero",
			php:  `<?php echo count(bind_int());`,
			want: "0",
		},
		{
			name: "is_array on model.Array",
			php:  `<?php echo is_array(bind_list_array()) ? "y" : "n";`,
			want: "y",
		},
		{
			name: "is_array on []string",
			php:  `<?php echo is_array(bind_list_strings()) ? "y" : "n";`,
			want: "y",
		},
		{
			name: "is_array on map",
			php:  `<?php echo is_array(bind_map()) ? "y" : "n";`,
			want: "y",
		},
		{
			name: "is_array on struct is false",
			php:  `<?php echo is_array(bind_record()) ? "y" : "n";`,
			want: "n",
		},
		{
			name: "implode over []string",
			php:  `<?php echo implode(",", bind_list_strings());`,
			want: "alpha,beta,gamma,delta,epsilon",
		},
		{
			name: "implode over model.Array",
			php:  `<?php echo implode(",", bind_list_array());`,
			want: "alpha,beta,gamma,delta,epsilon",
		},
		{
			name: "in_array over []string",
			php:  `<?php echo in_array("gamma", bind_list_strings()) ? "y" : "n";`,
			want: "y",
		},
		{
			name: "in_array miss over []string",
			php:  `<?php echo in_array("zeta", bind_list_strings()) ? "y" : "n";`,
			want: "n",
		},
		{
			name: "array_values over []string",
			php:  `<?php echo implode(",", array_values(bind_list_strings()));`,
			want: "alpha,beta,gamma,delta,epsilon",
		},
		{
			name: "array_keys over map",
			php:  `<?php $k = array_keys(bind_map()); echo count($k);`,
			want: "2",
		},
		{
			name: "array_merge accepts a slice",
			php:  `<?php echo implode(",", array_merge(bind_list_strings(), array("zeta")));`,
			want: "alpha,beta,gamma,delta,epsilon,zeta",
		},
		{
			name: "array_slice over []string",
			php:  `<?php echo implode(",", array_slice(bind_list_strings(), 1, 2));`,
			want: "beta,gamma",
		},
		{
			name: "json_encode a []string is a JSON list",
			php:  `<?php echo json_encode(bind_list_strings());`,
			want: `["alpha","beta","gamma","delta","epsilon"]`,
		},
		{
			name: "json_encode a model.Array list is a JSON list",
			php:  `<?php echo json_encode(bind_list_array());`,
			want: `["alpha","beta","gamma","delta","epsilon"]`,
		},
		{
			name: "json_encode a map is a JSON object",
			php:  `<?php echo json_encode(bind_map());`,
			want: `{"id":1,"name":"alpha"}`,
		},
		{
			name: "array_map over []string returns a list",
			php:  `<?php echo implode(",", array_map(function ($v) { return strtoupper($v); }, bind_list_strings()));`,
			want: "ALPHA,BETA,GAMMA,DELTA,EPSILON",
		},
		{
			name: "usort sorts a returned slice in place",
			php:  `<?php $l = bind_list_any(); usort($l, function ($a, $b) { return bind_compare_desc($a, $b); }); echo implode(",", $l);`,
			want: "gamma,epsilon,delta,beta,alpha",
		},
		{
			name: "str_replace with a slice of needles",
			php:  `<?php echo str_replace(bind_list_strings(), "x", "alpha-beta-zeta");`,
			want: "x-x-zeta",
		},
		{
			name: "preg_match fills matches as an indexable list",
			php:  `<?php preg_match("/(\\w+):(\\w+)/", "key:value", $m); echo count($m) . $m[1] . $m[2];`,
			want: "3keyvalue",
		},
		{
			name: "preg_match_all fills grouped columns",
			php:  `<?php preg_match_all("/(\\w)=(\\d)/", "a=1 b=2", $m); echo implode(",", $m[1]) . "/" . implode(",", $m[2]);`,
			want: "a,b/1,2",
		},
		{
			name: "func_get_args is indexable and countable",
			php:  `<?php function f() { $a = func_get_args(); return count($a) . $a[1]; } echo f("x", "y");`,
			want: "2y",
		},
		{
			name: "array_splice still needs a real array",
			php:  `<?php try { array_splice(bind_list_strings(), 1); } catch (Exception $e) { echo "caught"; }`,
			want: "caught",
		},
		{
			name: "array_splice on an array returns the removed list",
			php:  `<?php $a = array("a", "b", "c"); $cut = array_splice($a, 1); echo implode(",", $cut) . "|" . implode(",", $a);`,
			want: "b,c|a",
		},
		{
			name: "empty slice is falsey like an empty array",
			php:  `<?php $e = array_slice(bind_list_strings(), 99); echo empty($e) ? "empty" : "full";`,
			want: "empty",
		},
		{
			name: "non-empty slice is truthy",
			php:  `<?php $l = bind_list_strings(); if ($l) { echo "truthy"; }`,
			want: "truthy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runBinding(t, tc.php); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBindingSliceCannotGrow documents the one operation a native slice cannot
// carry: `$a[] =` append. A slice cannot grow through the interface value
// holding it, so a binding whose result the script appends to must return a
// *model.Array. Writing an existing element is fine (see below).
func TestBindingSliceCannotGrow(t *testing.T) {
	prog, err := parser.Parse(`<?php $l = bind_list_strings(); $l[] = "zeta";`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := newBindingRuntime(&out)
	err = rt.Run(prog)
	if err == nil {
		t.Fatal("appending to a returned slice should fail; if this now works, the guidance in bindings.go needs updating")
	}
	if !strings.Contains(err.Error(), "cannot append") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestBindingCollectionsAreWritableInPlace covers the writes that do work:
// replacing an element of a returned slice, and adding or replacing a key of a
// returned map. Both are reference writes the script observes, matching how a
// returned *model.Array behaves.
func TestBindingCollectionsAreWritableInPlace(t *testing.T) {
	cases := []struct {
		name string
		php  string
		want string
	}{
		{
			name: "replace a slice element",
			php:  `<?php $l = bind_list_any(); $l[1] = "BETA"; echo implode(",", $l);`,
			want: "alpha,BETA,gamma,delta,epsilon",
		},
		{
			name: "replace a map value",
			php:  `<?php $m = bind_map(); $m["name"] = "omega"; echo $m["name"];`,
			want: "omega",
		},
		{
			name: "add a map key",
			php:  `<?php $m = bind_map(); $m["extra"] = 9; echo $m["extra"] . ":" . count($m);`,
			want: "9:3",
		},
		{
			name: "compound assignment into a map",
			php:  `<?php $m = bind_map(); $m["name"] .= "!"; echo $m["name"];`,
			want: "alpha!",
		},
		{
			name: "write a column of a returned row",
			php:  `<?php $rows = bind_rows_maps(); foreach ($rows as $r) { $r["name"] = "x"; } echo $rows[0]["name"];`,
			want: "x",
		},
		{
			name: "write a field of a returned struct pointer",
			php:  `<?php $r = bind_record(); $r->name = "omega"; echo $r->name;`,
			want: "omega",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runBinding(t, tc.php); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBindingListDestructuring covers list($a, $b) = over a native slice, the
// shape explode() returns.
func TestBindingListDestructuring(t *testing.T) {
	got := runBinding(t, `<?php list($a, $b) = explode(" as ", "rows as row"); echo $a . "|" . $b;`)
	if want := "rows|row"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// allocation benchmarks
// ---------------------------------------------------------------------------

// The construction benchmarks measure the value itself, with no VM involved:
// this is the cost a binding pays for choosing a return shape.

func BenchmarkConstructModelArray(b *testing.B) {
	words := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	b.ReportAllocs()
	for b.Loop() {
		out := model.NewArray()
		for _, w := range words {
			out.Append(w)
		}
		_ = out
	}
}

func BenchmarkConstructStringSlice(b *testing.B) {
	words := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	b.ReportAllocs()
	for b.Loop() {
		out := make([]string, len(words))
		copy(out, words)
		_ = out
	}
}

func BenchmarkConstructAnySlice(b *testing.B) {
	words := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	b.ReportAllocs()
	for b.Loop() {
		out := make([]any, 0, len(words))
		for _, w := range words {
			out = append(out, w)
		}
		_ = out
	}
}

func BenchmarkConstructStringMap(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		out := map[string]any{"id": int64(1), "name": "alpha"}
		_ = out
	}
}

func BenchmarkConstructRowsModelArray(b *testing.B) {
	words := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	b.ReportAllocs()
	for b.Loop() {
		rows := model.NewArray()
		for i, w := range words {
			row := model.NewArray()
			row.Set("id", int64(i))
			row.Set("name", w)
			rows.Append(row)
		}
		_ = rows
	}
}

func BenchmarkConstructRowsMaps(b *testing.B) {
	words := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	b.ReportAllocs()
	for b.Loop() {
		rows := make([]map[string]any, 0, len(words))
		for i, w := range words {
			rows = append(rows, map[string]any{"id": int64(i), "name": w})
		}
		_ = rows
	}
}

// The call benchmarks add the reflection return path (runner.invokeAny plus
// firstReturn), which is what actually differs between a concrete return type
// and `any`.

// benchmarkCall drives a binding through the same reflection sequence the
// runtime uses (reflect.Value.Call followed by Interface() on each result, see
// runner.invokeAny and runner.firstReturn), isolating the return path from the
// rest of the VM.
func benchmarkCall(b *testing.B, name string, args ...reflect.Value) {
	b.Helper()
	fn := tests.BindingFunc(name)
	if fn == nil {
		b.Fatalf("binding %q is not defined", name)
	}
	rv := reflect.ValueOf(fn)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var result any
		for _, o := range rv.Call(args) {
			if o.Type() == errorType {
				continue
			}
			if result == nil {
				result = o.Interface()
			}
		}
		_ = result
	}
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

func BenchmarkCallListModelArray(b *testing.B) { benchmarkCall(b, "bind_list_array") }

func BenchmarkCallListStrings(b *testing.B) { benchmarkCall(b, "bind_list_strings") }

func BenchmarkCallListAny(b *testing.B) { benchmarkCall(b, "bind_list_any") }

func BenchmarkCallListShared(b *testing.B) { benchmarkCall(b, "bind_list_shared") }

func BenchmarkCallAnyStrings(b *testing.B) { benchmarkCall(b, "bind_any_strings") }

func BenchmarkCallAnyModelArray(b *testing.B) { benchmarkCall(b, "bind_any_array") }

func BenchmarkCallRowsModelArray(b *testing.B) { benchmarkCall(b, "bind_rows_array") }

func BenchmarkCallRowsMaps(b *testing.B) { benchmarkCall(b, "bind_rows_maps") }

func BenchmarkCallRecordPointer(b *testing.B) { benchmarkCall(b, "bind_record") }

func BenchmarkCallRecordValue(b *testing.B) { benchmarkCall(b, "bind_record_value") }

func BenchmarkCallObject(b *testing.B) { benchmarkCall(b, "bind_object") }

func BenchmarkCallInt(b *testing.B) { benchmarkCall(b, "bind_int") }

func BenchmarkCallIntAny(b *testing.B) { benchmarkCall(b, "bind_int_any") }

func BenchmarkCallSmallInt(b *testing.B) { benchmarkCall(b, "bind_small_int") }

func BenchmarkCallString(b *testing.B) { benchmarkCall(b, "bind_string") }

func BenchmarkCallBool(b *testing.B) { benchmarkCall(b, "bind_bool") }

// The script benchmarks are end-to-end: call the binding from PHP and iterate
// the result. This is the number that matters for a real template render,
// where the VM's own per-statement cost sits alongside the binding's.

func benchmarkScript(b *testing.B, src string) {
	b.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := newBindingRuntime(&out)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out.Reset()
		if err := rt.Run(prog); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScriptForeachModelArray(b *testing.B) {
	benchmarkScript(b, `<?php foreach (bind_list_array() as $v) { echo $v; }`)
}

func BenchmarkScriptForeachStrings(b *testing.B) {
	benchmarkScript(b, `<?php foreach (bind_list_strings() as $v) { echo $v; }`)
}

func BenchmarkScriptForeachShared(b *testing.B) {
	benchmarkScript(b, `<?php foreach (bind_list_shared() as $v) { echo $v; }`)
}

func BenchmarkScriptRowsModelArray(b *testing.B) {
	benchmarkScript(b, `<?php foreach (bind_rows_array() as $r) { echo $r["name"]; }`)
}

func BenchmarkScriptRowsMaps(b *testing.B) {
	benchmarkScript(b, `<?php foreach (bind_rows_maps() as $r) { echo $r["name"]; }`)
}

func BenchmarkScriptRecords(b *testing.B) {
	benchmarkScript(b, `<?php foreach (bind_records() as $r) { echo $r->name; }`)
}

// Explode is the stdlib function templates exercise most. These bracket the
// change: the legacy binding is the *model.Array implementation stdlib used to
// have, the native one is what it returns now, and the third runs the real
// stdlib explode() so the two stay honest.

func BenchmarkCallExplodeLegacy(b *testing.B) {
	benchmarkCall(b, "bind_explode_legacy", reflect.ValueOf(","), reflect.ValueOf("a,b,c,d,e"))
}

func BenchmarkCallExplodeNative(b *testing.B) {
	benchmarkCall(b, "bind_explode_native", reflect.ValueOf(","), reflect.ValueOf("a,b,c,d,e"))
}

func BenchmarkScriptExplodeLegacy(b *testing.B) {
	benchmarkScript(b, `<?php foreach (bind_explode_legacy(",", "a,b,c,d,e") as $v) { echo $v; }`)
}

func BenchmarkScriptExplode(b *testing.B) {
	benchmarkScript(b, `<?php foreach (explode(",", "a,b,c,d,e") as $v) { echo $v; }`)
}

// A template's real shape: split a line, then join part of it back. Every stdlib
// call in the chain now passes native slices through instead of rebuilding an
// *model.Array at each step.
func BenchmarkScriptExplodeImplodeChain(b *testing.B) {
	benchmarkScript(b, `<?php echo implode("-", array_slice(explode(",", "a,b,c,d,e"), 1, 3));`)
}

func BenchmarkScriptPregMatch(b *testing.B) {
	benchmarkScript(b, `<?php preg_match("/^(\\w+):(\\w+)$/", "key:value", $m); echo $m[1] . $m[2];`)
}

// ---------------------------------------------------------------------------
// what a return shape is competing with
// ---------------------------------------------------------------------------

// The script benchmarks above are dominated by a cost that has nothing to do
// with return shapes: runner.baseEnv rebuilds the expression environment on
// every Eval, allocating one closure per registered function. These two run the
// same script against runtimes with different function-table sizes, so the
// per-registration cost is visible next to the per-return cost.

func benchmarkScriptWith(b *testing.B, src string, register func(*runner.Runtime)) {
	b.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	register(rt)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out.Reset()
		if err := rt.Run(prog); err != nil {
			b.Fatal(err)
		}
	}
}

const envScript = `<?php foreach (bind_list_strings() as $v) { echo $v; }`

// BenchmarkScriptEnvFullStdlib registers the whole stdlib, as a host normally
// does: roughly eighty functions.
func BenchmarkScriptEnvFullStdlib(b *testing.B) {
	benchmarkScriptWith(b, envScript, func(rt *runner.Runtime) {
		stdlib.Register(rt, tests.RegisterBindings)
	})
}

// BenchmarkScriptEnvMinimal registers only the binding the script calls. The
// difference against the full stdlib is the price of the function table itself.
func BenchmarkScriptEnvMinimal(b *testing.B) {
	benchmarkScriptWith(b, envScript, func(rt *runner.Runtime) {
		rt.RegisterFunc("bind_list_strings", tests.BindingFunc("bind_list_strings"))
	})
}
