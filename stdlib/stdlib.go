// Package stdlib provides the forwarded "bring your own standard library" shims
// (the README's register_function mechanism). PHP's stdlib is not reimplemented
// in the VM; instead a curated set of Go functions is registered on a Runtime so
// transpiled PHP can call them by name. This set is sized to run the minitpl
// template engine (the T1 compatibility target).
package stdlib

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
)

// Register installs the pure (non-filesystem) shims, PHP constants, every
// binding contributed by an imported binding package (see
// runner.RegisterBinding and imports.go), and any additional bindings passed by
// the caller. Use RegisterFS to add filesystem IO bound to a root directory.
func Register(rt *runner.Runtime, bindings ...func(*runner.Runtime)) {
	rt.RegisterConstructor("Exception", NewException)

	registerStrings(rt)
	registerArrays(rt)
	registerJSON(rt)
	registerRegex(rt)
	registerLang(rt)
	registerTokenizer(rt)
	registerEnvironment(rt)

	for _, register := range runner.Bindings() {
		register(rt)
	}
	for _, register := range bindings {
		register(rt)
	}
}

func registerEnvironment(rt *runner.Runtime) {
	rt.RegisterFunc("putenv", func(name string, values ...string) bool {
		if len(values) > 0 {
			rt.Env[name] = values[0]
			return true
		}
		if key, value, ok := strings.Cut(name, "="); ok {
			rt.Env[key] = value
		} else {
			delete(rt.Env, name)
		}
		return true
	})
	rt.RegisterFunc("getenv", func(name string) any {
		value, ok := rt.Env[name]
		if !ok {
			return false
		}
		return value
	})
}

// ---------------------------------------------------------------------------
// strings
// ---------------------------------------------------------------------------

func registerStrings(rt *runner.Runtime) {
	rt.RegisterFunc("strlen", func(s string) int64 { return int64(len(s)) })
	rt.RegisterFunc("strtoupper", strings.ToUpper)
	rt.RegisterFunc("strtolower", strings.ToLower)
	rt.RegisterFunc("trim", phpTrim(strings.Trim, " \t\n\r\x00\x0B"))
	rt.RegisterFunc("rtrim", phpTrim(strings.TrimRight, " \t\n\r\x00\x0B"))
	rt.RegisterFunc("ltrim", phpTrim(strings.TrimLeft, " \t\n\r\x00\x0B"))

	rt.RegisterFunc("substr", phpSubstr)
	rt.RegisterFunc("strpos", func(haystack, needle string) any {
		i := strings.Index(haystack, needle)
		if i < 0 {
			return false
		}
		return int64(i)
	})
	rt.RegisterFunc("strstr", func(haystack, needle string) any {
		i := strings.Index(haystack, needle)
		if i < 0 {
			return false
		}
		return haystack[i:]
	})
	rt.RegisterFunc("str_replace", phpStrReplace)
	rt.RegisterFunc("str_repeat", func(s string, n int64) string { return strings.Repeat(s, int(n)) })
	rt.RegisterFunc("implode", phpImplode)
	rt.RegisterFunc("explode", phpExplode)
	rt.RegisterFunc("htmlspecialchars", func(s string, _ ...any) string {
		return htmlSpecialCharsReplacer.Replace(s)
	})
	rt.RegisterFunc("sprintf", phpSprintf)
	rt.RegisterFunc("crc32", phpCRC32)
}

// htmlSpecialCharsReplacer is built once: a strings.Replacer compiles a trie
// over its pairs, and htmlspecialchars runs once per template variable, so
// constructing it inside the binding paid for that trie on every call.
var htmlSpecialCharsReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#039;",
)

// crc32IEEETable is the standard reflected IEEE table, built once.
var crc32IEEETable = crc32.MakeTable(crc32.IEEE)

// phpCRC32 checksums a string without the []byte(s) conversion the previous
// implementation paid on every call. Escape analysis confirmed the conversion
// escaped ("stdlib.go: ([]byte)(s) escapes to heap"): crc32.ChecksumIEEE's
// argument leaks into the slicing-by-8 and hardware paths, so every crc32()
// allocated and copied its input. Short strings — every practical use, since
// crc32 keys and etags are short — run the table loop in place and allocate
// nothing. Longer ones keep the standard library's implementation, which
// processes eight bytes at a time and amortises the copy over the input.
func phpCRC32(s string) int64 {
	if len(s) > crc32NativeThreshold {
		return int64(crc32.ChecksumIEEE([]byte(s)))
	}
	crc := ^uint32(0)
	for i := 0; i < len(s); i++ {
		crc = crc32IEEETable[byte(crc)^s[i]] ^ (crc >> 8)
	}
	return int64(^crc)
}

// crc32NativeThreshold is where the table loop stops winning outright. Below
// it the loop is several times faster than allocating a copy (8 bytes: 38ns/0
// allocs against 105ns/1 alloc); at 256 the two are level on this hardware.
// See BenchmarkCRC32.
const crc32NativeThreshold = 256

// phpTrim adapts strings.Trim*-style functions to PHP's optional charlist arg.
func phpTrim(fn func(string, string) string, def string) func(string, ...string) string {
	return func(s string, chars ...string) string {
		cut := def
		if len(chars) > 0 {
			cut = chars[0]
		}
		return fn(s, cut)
	}
}

// phpSubstr implements substr($s, $start[, $length]) with PHP's negative
// offset/length semantics.
func phpSubstr(s string, start int64, length ...int64) string {
	n := int64(len(s))
	if start < 0 {
		start += n
		if start < 0 {
			start = 0
		}
	}
	if start > n {
		return ""
	}
	end := n
	if len(length) > 0 {
		l := length[0]
		if l < 0 {
			end = n + l
		} else {
			end = start + l
		}
	}
	if end > n {
		end = n
	}
	if end < start {
		return ""
	}
	return s[start:end]
}

// phpStrReplace implements str_replace where search may be a string or a
// collection (with a scalar or collection replacement), and subject is a string.
func phpStrReplace(search, replace, subject any) string {
	out := toString(subject)
	if model.IsCollection(search) {
		replIsList := model.IsCollection(replace)
		repl := arrStrings(replace)
		// A scalar replacement converts once, not once per search term.
		scalar := ""
		if !replIsList {
			scalar = toString(replace)
		}
		for i, s := range arrStrings(search) {
			r := scalar
			if replIsList {
				r = ""
				if i < len(repl) {
					r = repl[i]
				}
			}
			out = strings.ReplaceAll(out, s, r)
		}
		return out
	}
	return strings.ReplaceAll(out, toString(search), toString(replace))
}

func phpImplode(a, b any) string {
	// implode($glue, $array) or implode($array).
	if model.IsCollection(a) {
		return strings.Join(arrStrings(a), "")
	}
	// A []string joins without the per-element conversion arrStrings would do.
	if parts, ok := b.([]string); ok {
		return strings.Join(parts, toString(a))
	}
	return strings.Join(arrStrings(b), toString(a))
}

// phpExplode returns the parts as a []string: strings.Split already allocated
// exactly that, so handing it straight to the VM costs nothing beyond the split
// itself. The VM indexes, iterates and destructures it like any array.
func phpExplode(delim, s string, limit ...int64) []string {
	parts := strings.Split(s, delim)
	if len(limit) > 0 && limit[0] > 0 && int64(len(parts)) > limit[0] {
		tail := strings.Join(parts[limit[0]-1:], delim)
		parts = parts[:limit[0]]
		parts[limit[0]-1] = tail
	}
	return parts
}

// phpSprintf implements a subset of sprintf: %s %d %u %% and width/precision
// pass-through to fmt where compatible.
func phpSprintf(format string, args ...any) string {
	// Go has no %u; rewrite to %d with the value coerced to unsigned.
	if strings.Contains(format, "%u") {
		format = strings.ReplaceAll(format, "%u", "%d")
		for i, a := range args {
			args[i] = uint64(toInt(a) & 0xFFFFFFFF)
		}
	}
	return fmt.Sprintf(format, args...)
}

// ---------------------------------------------------------------------------
// arrays
// ---------------------------------------------------------------------------

// The array shims take `any` rather than *model.Array and read their input
// through model.RangeValues, so a script can pass either a PHP array or the
// native Go slice/map a binding returned. Those that build a fresh list return
// a []any (one allocation, presized) instead of an *model.Array (a struct, a
// map[any]any, a growing key slice and an interface box per key). The ones that
// preserve or merge keys still return *model.Array, because only *model.Array
// carries PHP's ordered hybrid-key semantics.
func registerArrays(rt *runner.Runtime) {
	rt.RegisterFunc("compact", phpCompact)
	rt.RegisterFunc("count", func(a any) int64 {
		n, _ := model.LenValues(a)
		return int64(n)
	})
	rt.RegisterFunc("in_array", func(needle, haystack any, _ ...any) bool {
		found := false
		model.RangeValues(haystack, func(_, v any) bool {
			if toString(v) == toString(needle) {
				found = true
				return false
			}
			return true
		})
		return found
	})
	rt.RegisterFunc("array_unique", func(a any, _ ...any) *model.Array {
		n, _ := model.LenValues(a)
		out := model.NewArraySize(n)
		seen := make(map[string]bool, n)
		model.RangeValues(a, func(k, v any) bool {
			s := toString(v)
			if !seen[s] {
				seen[s] = true
				out.Set(k, v)
			}
			return true
		})
		return out
	})
	rt.RegisterFunc("array_merge", phpArrayMerge)
	rt.RegisterFunc("array_keys", func(a any) []any {
		n, _ := model.LenValues(a)
		out := make([]any, 0, n)
		model.RangeValues(a, func(k, _ any) bool { out = append(out, k); return true })
		return out
	})
	rt.RegisterFunc("array_values", func(a any) []any {
		n, _ := model.LenValues(a)
		out := make([]any, 0, n)
		model.RangeValues(a, func(_, v any) bool { out = append(out, v); return true })
		return out
	})
	rt.RegisterFunc("array_slice", phpArraySlice)
	rt.RegisterFunc("array_splice", phpArraySplice)
	rt.RegisterFunc("array_map", phpArrayMap)
	rt.RegisterFunc("usort", phpUsort)
	rt.RegisterFunc("sort", phpSort)
	rt.RegisterFunc("rsort", phpRsort)
}

// phpArrayMerge implements array_merge. PHP renumbers integer keys and lets
// later string keys win.
//
// This returns an *model.Array even when every input is a list. Returning a
// presized []any for that case is tempting — it is the shape
// `call_user_func_array($fn, array_merge(...))` forwards with no copy — but
// rule 4 of docs/allocation-performance.md reserves *model.Array for values
// the script appends to, and `$x = array_merge($a, $b); $x[] = "z"` is
// ordinary PHP that a Go slice cannot serve (a slice cannot grow through the
// interface value holding it). Since model.Array gained its list mode, a
// merged list costs one struct allocation more than the slice and no map at
// all, so the appendability is close to free. Measured: 8 allocs either way.
func phpArrayMerge(arrs ...any) any {
	size := 0
	for _, a := range arrs {
		n, _ := model.LenValues(a)
		size += n
	}

	out := model.NewArraySize(size)
	for _, a := range arrs {
		model.RangeValues(a, func(k, v any) bool {
			if _, isInt := k.(int64); isInt {
				out.Append(v)
			} else {
				out.Set(k, v)
			}
			return true
		})
	}
	return out
}

// phpCompact builds an associative map from variables in the calling scope.
// Names that do not exist are omitted, matching PHP's compact().
func phpCompact(ctx context.Context, names ...string) map[string]any {
	out := make(map[string]any, len(names))
	scope, ok := runner.ScopeFromContext(ctx)
	if !ok {
		return out
	}
	for _, name := range names {
		if value, ok := scope.Get(name); ok {
			out[name] = value
		}
	}
	return out
}

// phpArraySplice removes (and optionally replaces) a run of elements, returning
// the removed ones as a []any. It resizes its argument, which a Go slice cannot
// do through the interface holding it, so this is one of the few shims that
// genuinely requires a *model.Array — passing anything else is an error rather
// than a silent no-op.
func phpArraySplice(target any, offset int64, optional ...any) ([]any, error) {
	a, ok := target.(*model.Array)
	if !ok || a == nil {
		return nil, fmt.Errorf("array_splice: expected an array, got %T", target)
	}

	vals := arrValues(a)
	n := len(vals)

	start := int(offset)
	if start < 0 {
		start += n
	}
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}

	end := n
	var replacement []any
	switch len(optional) {
	case 0:
		// length omitted: remove through end of array
	case 1:
		end = phpArraySpliceEnd(start, n, optional[0])
	case 2:
		end = phpArraySpliceEnd(start, n, optional[0])
		replacement = phpArraySpliceReplacement(optional[1])
	default:
		end = phpArraySpliceEnd(start, n, optional[0])
		replacement = phpArraySpliceReplacement(optional[1])
	}

	removed := make([]any, end-start)
	copy(removed, vals[start:end])

	out := make([]any, 0, start+len(replacement)+(n-end))
	out = append(out, vals[:start]...)
	out = append(out, replacement...)
	out = append(out, vals[end:]...)

	a.Clear()
	for _, v := range out {
		a.Append(v)
	}

	return removed, nil
}

func phpArraySpliceEnd(start, n int, lengthArg any) int {
	if lengthArg == nil {
		return n
	}
	l := int(toInt(lengthArg))
	if l >= 0 {
		end := start + l
		if end > n {
			end = n
		}
		return end
	}
	end := n + l
	if end < start {
		end = start
	}
	return end
}

func phpArraySpliceReplacement(rep any) []any {
	if rep == nil {
		return nil
	}
	if model.IsCollection(rep) {
		return arrValues(rep)
	}
	return []any{rep}
}

// phpArraySlice returns the selected run as a []any. PHP's array_slice
// reindexes integer keys, which this shim has always done for every key, so a
// list is a faithful representation of the result.
func phpArraySlice(a any, offset int64, length ...int64) []any {
	vals := arrValues(a)
	start := int(offset)
	if start < 0 {
		start += len(vals)
	}
	if start < 0 {
		start = 0
	}
	if start > len(vals) {
		return nil
	}
	end := len(vals)
	if len(length) > 0 {
		end = start + int(length[0])
		if end < start {
			end = start
		}
		if end > len(vals) {
			end = len(vals)
		}
	}
	out := make([]any, end-start)
	copy(out, vals[start:end])
	return out
}

func phpArrayMap(fn func(...any) (any, error), a any) ([]any, error) {
	n, _ := model.LenValues(a)
	out := make([]any, 0, n)
	var mapErr error
	model.RangeValues(a, func(_, v any) bool {
		mapped, err := fn(v)
		if err != nil {
			mapErr = err
			return false
		}
		out = append(out, mapped)
		return true
	})
	if mapErr != nil {
		return nil, mapErr
	}
	return out, nil
}

// phpUsort sorts values in place using cmp, reindexing with integer keys (PHP
// semantics). cmp is the env-adapted comparator: func(...any)(any,error).
func phpUsort(a any, cmp func(...any) (any, error)) bool {
	return sortValues(a, func(x, y any) bool {
		r, err := cmp(x, y)
		if err != nil {
			return false
		}
		return toInt(r) < 0
	})
}

// phpSort orders a list with PHP's default comparison: numerically when both
// sides are numeric, by string otherwise. Like sort(), it discards the keys and
// reindexes the result from zero.
func phpSort(a any) bool {
	return sortValues(a, sortLess)
}

// phpRsort is phpSort in descending order.
func phpRsort(a any) bool {
	return sortValues(a, func(x, y any) bool { return sortLess(y, x) })
}

// sortLess is PHP's default comparison: two numbers compare as numbers, any
// other pair compares as strings.
func sortLess(x, y any) bool {
	xn, xok := numericValue(x)
	yn, yok := numericValue(y)
	if xok && yok {
		return xn < yn
	}
	return toString(x) < toString(y)
}

// numericValue returns v as a float64 if it is one of the runtime's number
// types. A numeric string is not one of them, so sort() orders "10" before "9"
// where PHP, which compares two numeric strings as numbers, does not.
func numericValue(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// sortValues reorders a list in place using less. A *model.Array is rebuilt as
// a 0-indexed list; a native Go slice is sorted through its backing array,
// which the script shares, so both are mutated in place as PHP expects.
func sortValues(a any, less func(x, y any) bool) bool {
	switch target := a.(type) {
	case nil:
		return false
	case *model.Array:
		if target == nil {
			return false
		}
		vals := arrValues(target)
		sort.SliceStable(vals, func(i, j int) bool { return less(vals[i], vals[j]) })
		target.Clear()
		for _, v := range vals {
			target.Append(v)
		}
		return true
	case []any:
		sort.SliceStable(target, func(i, j int) bool { return less(target[i], target[j]) })
		return true
	}

	rv := reflect.ValueOf(a)
	if rv.Kind() != reflect.Slice {
		return false
	}
	// rv indexes the same backing array sort.SliceStable swaps, so reads stay
	// consistent with the ongoing sort.
	sort.SliceStable(a, func(i, j int) bool {
		return less(rv.Index(i).Interface(), rv.Index(j).Interface())
	})
	return true
}

// ---------------------------------------------------------------------------
// json
// ---------------------------------------------------------------------------

func registerJSON(rt *runner.Runtime) {
	rt.RegisterFunc("json_encode", phpJSONEncode)
	rt.RegisterFunc("json_decode", phpJSONDecode)
}

func phpJSONEncode(v any) (any, error) {
	b, err := json.Marshal(jsonEncodeValue(v))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func phpJSONDecode(s string) (any, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return jsonDecodeValue(v), nil
}

// jsonEncodeValue rewrites the value model into something encoding/json
// understands. Native Go collections are already encodable, so they are only
// walked when they might contain an *model.Array or *model.Object; scalars and
// []string pass through untouched.
func jsonEncodeValue(v any) any {
	switch x := v.(type) {
	case nil, string, bool, int64, int, float64, []string:
		return v
	case *model.Array:
		if x == nil {
			return nil
		}
		if arrayIsList(x) {
			out := make([]any, 0, x.Len())
			x.Range(func(_, v any) bool {
				out = append(out, jsonEncodeValue(v))
				return true
			})
			return out
		}
		out := make(map[string]any, x.Len())
		x.Range(func(k, v any) bool {
			out[toString(k)] = jsonEncodeValue(v)
			return true
		})
		return out
	case *model.Object:
		if x == nil {
			return nil
		}
		out := make(map[string]any, len(x.Props))
		for k, v := range x.Props {
			out[k] = jsonEncodeValue(v)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = jsonEncodeValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = jsonEncodeValue(item)
		}
		return out
	default:
		return v
	}
}

// arrayIsList reports whether an *model.Array's keys are the dense int64
// sequence 0..n-1, i.e. whether it is a PHP list.
func arrayIsList(a *model.Array) bool {
	expect := int64(0)
	isList := true
	a.Range(func(k, _ any) bool {
		if i, ok := k.(int64); !ok || i != expect {
			isList = false
			return false
		}
		expect++
		return true
	})
	return isList
}

func jsonDecodeValue(v any) any {
	switch x := v.(type) {
	case []any:
		// A JSON array is positional, so a []any loses nothing and reuses the
		// slice the decoder already allocated.
		for i, item := range x {
			x[i] = jsonDecodeValue(item)
		}
		return x
	case map[string]any:
		// A JSON object becomes an *model.Array. The decoder's map has already
		// lost the document's key order, but an *model.Array at least fixes one
		// order for the value's lifetime, so iterating a decoded object twice
		// renders the same output twice. Handing the map through would make
		// every foreach re-randomise.
		out := model.NewArraySize(len(x))
		for k, v := range x {
			out.Set(k, jsonDecodeValue(v))
		}
		return out
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return string(x)
	default:
		return v
	}
}

// ---------------------------------------------------------------------------
// language constructs exposed as functions
// ---------------------------------------------------------------------------

func registerLang(rt *runner.Runtime) {
	rt.SetConst("DIRECTORY_SEPARATOR", string(os.PathSeparator))
	rt.SetConst("PATH_SEPARATOR", string(os.PathListSeparator))
	rt.SetConst("STDIN", rt.Stdin())
	rt.RegisterFunc("stream_get_contents", func(stream io.Reader, _ ...any) (string, error) {
		v, err := io.ReadAll(stream)
		if err != nil {
			return "", err
		}
		return string(v), nil
	})
	rt.RegisterFunc("spl_autoload_register", func(args ...any) (bool, error) {
		var callback any = rt.SPLAutoload
		if len(args) > 0 && args[0] != nil {
			callback = args[0]
		}
		prepend := len(args) > 2 && truthy(args[2])
		rt.RegisterAutoloader(callback, prepend)
		return true, nil
	})
	rt.RegisterFunc("spl_autoload", func(class string, _ ...any) error {
		return rt.SPLAutoload(class)
	})
	rt.RegisterFunc("class_exists", func(name string, autoload ...bool) (bool, error) {
		load := true
		if len(autoload) > 0 {
			load = autoload[0]
		}
		return rt.ClassExists(name, load)
	})
	rt.RegisterFunc("set_include_path", rt.SetIncludePath)
	rt.RegisterFunc("get_include_path", rt.IncludePath)
	// The introspection shims keep returning *model.Array: their whole value is
	// a stable, name-sorted listing, which a Go map cannot express.
	rt.RegisterFunc("get_defined_constants", func(categorize ...bool) *model.Array {
		defined := rt.DefinedConstants()
		constants := model.NewArraySize(len(defined))
		names := make([]string, 0, len(defined))
		for name := range defined {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			constants.Set(name, defined[name])
		}
		if len(categorize) > 0 && categorize[0] {
			grouped := model.NewArraySize(1)
			grouped.Set("Core", constants)
			return grouped
		}
		return constants
	})
	rt.RegisterFunc("get_defined_functions", func(_ ...bool) map[string][]string {
		internal, user := rt.DefinedFunctions()
		return map[string][]string{"internal": internal, "user": user}
	})
	rt.RegisterFunc("get_defined_vars", func(ctx context.Context) *model.Array {
		scope, ok := runner.ScopeFromContext(ctx)
		if !ok {
			return model.NewArray()
		}
		defined := scope.DefinedVars()
		vars := model.NewArraySize(len(defined))
		names := make([]string, 0, len(defined))
		for name := range defined {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			vars.Set(name, defined[name])
		}
		return vars
	})
	rt.RegisterFunc("get_declared_classes", func() []string {
		return rt.DeclaredClasses()
	})
	rt.RegisterFunc("phpinfo", func(_ ...any) (bool, error) {
		return true, rt.PHPInfo()
	})
	rt.RegisterFunc("php_sapi_name", func() string {
		return rt.SAPI()
	})
	rt.RegisterFunc("isset", func(args ...any) bool {
		for _, a := range args {
			if a == nil {
				return false
			}
		}
		return true
	})
	rt.RegisterFunc("empty", func(a any) bool { return !truthy(a) })
	// A binding's []string is as much a PHP array as an *model.Array is, so
	// is_array() answers for the whole value model, not one Go type.
	rt.RegisterFunc("is_array", model.IsCollection)
	rt.RegisterFunc("is_int", func(a any) bool {
		switch a.(type) {
		case int, int64:
			return true
		}
		return false
	})
	rt.RegisterFunc("is_string", func(a any) bool { _, ok := a.(string); return ok })
	rt.RegisterFunc("is_bool", func(a any) bool { _, ok := a.(bool); return ok })
	rt.RegisterFunc("is_object", func(a any) bool { _, ok := a.(*model.Object); return ok })
	rt.RegisterFunc("get_included_files", func() []string { return rt.IncludedFiles() })
	rt.RegisterFunc("is_numeric", func(a any) bool {
		switch a.(type) {
		case int64, int, float64:
			return true
		}
		return false
	})
	rt.RegisterFunc("call_user_func_array", phpCallUserFuncArray)
	rt.RegisterFunc("function_exists", func(string) bool { return false })
	// PHP's exit/die takes either a status or a message: a string argument is
	// printed and the script exits with status 0, an integer sets the status.
	terminate := func(code ...any) (any, error) {
		status := 0
		if len(code) > 0 {
			if message, ok := code[0].(string); ok {
				if _, err := io.WriteString(rt.Output(), message); err != nil {
					return nil, err
				}
			} else {
				status = int(toInt(code[0]))
			}
		}
		return nil, rt.Exit(status)
	}
	rt.RegisterFunc("exit", terminate)
	rt.RegisterFunc("die", terminate)
}

func phpCallUserFuncArray(fn func(...any) (any, error), args any) (any, error) {
	// A []any argument list is already the shape fn wants, so the common case
	// (array_merge's output, func_get_args) forwards without a copy.
	if callArgs, ok := args.([]any); ok {
		return fn(callArgs...)
	}
	return fn(arrValues(args)...)
}

// ---------------------------------------------------------------------------
// tokenizer + constants
// ---------------------------------------------------------------------------

func registerTokenizer(rt *runner.Runtime) {
	rt.RegisterFunc("token_get_all", parser.TokenGetAll)
	rt.RegisterFunc("token_name", func(id int64) string { return parser.TokenName(int(id)) })

	// Bare T_* constants used by Compiler::_split_exp. They are constants, not
	// globals, so they resolve inside methods/functions too (PHP semantics).
	rt.SetConst("T_VARIABLE", int64(parser.T_VARIABLE))
	rt.SetConst("T_OBJECT_OPERATOR", int64(parser.T_OBJECT_OPERATOR))
	rt.SetConst("T_STRING", int64(parser.T_STRING))
}

// ---------------------------------------------------------------------------
// shared coercion helpers (kept local to the shim layer)
// ---------------------------------------------------------------------------

func toString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return ""
	case int64:
		// strconv formats in place; fmt.Sprintf would box x into an any
		// (an allocation of its own for values over 255) and run the
		// formatter. toString sits under implode, str_replace, in_array and
		// array_unique, so it is worth the two lines.
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		// %v for a float64 is 'g' with the shortest representation that
		// round-trips, which is exactly what FormatFloat(-1) produces.
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func toInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		return leadingInt(x)
	default:
		return 0
	}
}

// leadingInt reads the integer prefix of s the way fmt.Sscanf(s, "%d", &n) did:
// leading whitespace and an optional sign, then digits, stopping at the first
// character that is not one ("12abc" is 12, "abc" is 0). Overflow yields 0, the
// value Sscanf left behind when it failed. Sscanf allocated a scan state, a
// reader and an argument box on every call; this reads the string in place.
func leadingInt(s string) int64 {
	i := 0
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			i++
			continue
		}
		break
	}
	negative := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		negative = s[i] == '-'
		i++
	}
	var n int64
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		digit := int64(s[i] - '0')
		if n > (math.MaxInt64-digit)/10 {
			return 0
		}
		n = n*10 + digit
	}
	if negative {
		return -n
	}
	return n
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != "" && x != "0"
	case int64:
		return x != 0
	case int:
		return x != 0
	case float64:
		return x != 0
	case *model.Array:
		return x.Len() > 0
	default:
		if n, ok := model.LenValues(v); ok {
			return n > 0
		}
		return true
	}
}

// arrStrings returns a collection's values as strings in order.
func arrStrings(a any) []string {
	if parts, ok := a.([]string); ok {
		return parts
	}
	n, _ := model.LenValues(a)
	if n == 0 {
		return nil
	}
	out := make([]string, 0, n)
	model.RangeValues(a, func(_, v any) bool { out = append(out, toString(v)); return true })
	return out
}

// arrValues returns a collection's values in order. A []any is returned as is:
// callers only read it, and the shims that do not (array_splice) copy first.
func arrValues(a any) []any {
	if vals, ok := a.([]any); ok {
		return vals
	}
	n, _ := model.LenValues(a)
	if n == 0 {
		return nil
	}
	out := make([]any, 0, n)
	model.RangeValues(a, func(_, v any) bool { out = append(out, v); return true })
	return out
}
