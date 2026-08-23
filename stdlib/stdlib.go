// Package stdlib provides the forwarded "bring your own standard library" shims
// (the README's register_function mechanism). PHP's stdlib is not reimplemented
// in the VM; instead a curated set of Go functions is registered on a Runtime so
// transpiled PHP can call them by name. This set is sized to run the minitpl
// template engine (the T1 compatibility target).
package stdlib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/internal/arrayi64"
	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
)

// Register installs the shims, PHP constants, every
// binding contributed by an imported binding package (see
// runner.RegisterBinding and imports.go), and any additional bindings passed by
// the caller. The filesystem shims come in that way too, rooted at the process
// working directory; use RegisterFS to bind them to another root.
func Register(rt *runner.Runtime, bindings ...func(*runner.Runtime)) {
	// Exception is PHP's base exception class; the SPL exception and Error classes are backed by the same type, so a catch cannot filter by subclass.
	rt.RegisterConstructor("Exception", NewException)

	registerStrings(rt)
	registerArrays(rt)
	registerJSON(rt)
	registerLang(rt)
	registerTokenizer(rt)
	registerEnvironment(rt)
	registerPlatform(rt)

	for _, register := range runner.Bindings() {
		register(rt)
	}
	for _, register := range bindings {
		register(rt)
	}
}

func registerEnvironment(rt *runner.Runtime) {
	// putenv sets an environment variable from a "NAME=value" string, or unsets a bare "NAME"; it always returns true.
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
	// getenv returns the value of the environment variable $name, or false when it is not set.
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
	// strlen returns the length of $str in bytes.
	rt.RegisterFunc("strlen", func(str string) int64 { return int64(len(str)) })
	// strtoupper returns $string uppercased; unlike PHP's ASCII-only mapping, non-ASCII letters are converted too.
	rt.RegisterFunc("strtoupper", strings.ToUpper)
	// strtolower returns $string lowercased; unlike PHP's ASCII-only mapping, non-ASCII letters are converted too.
	rt.RegisterFunc("strtolower", strings.ToLower)
	// trim strips whitespace, or the characters listed in $characters, from both ends of $string.
	rt.RegisterFunc("trim", phpTrim(strings.Trim, " \t\n\r\x00\x0B"))
	// rtrim strips whitespace, or the characters listed in $characters, from the end of $string.
	rt.RegisterFunc("rtrim", phpTrim(strings.TrimRight, " \t\n\r\x00\x0B"))
	// ltrim strips whitespace, or the characters listed in $characters, from the start of $string.
	rt.RegisterFunc("ltrim", phpTrim(strings.TrimLeft, " \t\n\r\x00\x0B"))

	rt.RegisterFunc("substr", phpSubstr)
	// strpos returns the byte offset of the first $needle in $haystack, or false when it does not occur; there is no $offset parameter.
	rt.RegisterFunc("strpos", func(haystack, needle string) any {
		i := strings.Index(haystack, needle)
		if i < 0 {
			return false
		}
		return int64(i)
	})
	// strstr returns $haystack from the first occurrence of $needle to the end, or false when it does not occur; there is no $before_needle parameter.
	rt.RegisterFunc("strstr", func(haystack, needle string) any {
		i := strings.Index(haystack, needle)
		if i < 0 {
			return false
		}
		return haystack[i:]
	})
	rt.RegisterFunc("str_replace", phpStrReplace)
	// str_repeat returns $str repeated $times times.
	rt.RegisterFunc("str_repeat", func(str string, times int64) string { return strings.Repeat(str, int(times)) })
	// implode returns the values of $array joined with $separator; with a single array argument the separator is "".
	rt.RegisterFunc("implode", phpImplode)
	// explode splits $str on $separator into a list; a positive $limit caps the parts, the last one holding the rest, and other limits are ignored.
	rt.RegisterFunc("explode", phpExplode)
	// htmlspecialchars escapes &, <, >, double and single quotes as HTML entities; the $flags and later arguments are accepted and ignored.
	rt.RegisterFunc("htmlspecialchars", func(s string, flags ...any) string {
		return htmlSpecialCharsReplacer.Replace(s)
	})
	rt.RegisterFunc("sprintf", phpSprintf)
	// crc32 returns the CRC-32 checksum of $str as an integer.
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

// phpCRC32 checksums a string without the []byte(str) conversion the previous
// implementation paid on every call. Escape analysis confirmed the conversion
// escaped ("stdlib.go: ([]byte)(s) escapes to heap"): crc32.ChecksumIEEE's
// argument leaks into the slicing-by-8 and hardware paths, so every crc32()
// allocated and copied its input. Short strings, every practical use since
// crc32 keys and etags are short, run the table loop in place and allocate
// nothing. Longer ones keep the standard library's implementation, which
// processes eight bytes at a time and amortises the copy over the input.
func phpCRC32(str string) int64 {
	if len(str) > crc32NativeThreshold {
		return int64(crc32.ChecksumIEEE([]byte(str)))
	}
	crc := ^uint32(0)
	for i := 0; i < len(str); i++ {
		crc = crc32IEEETable[byte(crc)^str[i]] ^ (crc >> 8)
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
	out := phpval.String(subject)
	if model.IsCollection(search) {
		replIsList := model.IsCollection(replace)
		repl := phpval.Strings(replace)
		// A scalar replacement converts once, not once per search term.
		scalar := ""
		if !replIsList {
			scalar = phpval.String(replace)
		}
		for i, s := range phpval.Strings(search) {
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
	return strings.ReplaceAll(out, phpval.String(search), phpval.String(replace))
}

func phpImplode(separator, array any) string {
	// implode($separator, $array) or implode($array), where the single
	// array argument arrives in $separator.
	if model.IsCollection(separator) {
		return strings.Join(phpval.Strings(separator), "")
	}
	// A []string joins without the per-element conversion phpval.Strings would do.
	if parts, ok := array.([]string); ok {
		return strings.Join(parts, phpval.String(separator))
	}
	return strings.Join(phpval.Strings(array), phpval.String(separator))
}

// phpExplode returns the parts as a []string: strings.Split already allocated
// exactly that, so handing it straight to the VM costs nothing beyond the split
// itself. The VM indexes, iterates and destructures it like any array.
func phpExplode(separator, str string, limit ...int64) []string {
	parts := strings.Split(str, separator)
	if len(limit) > 0 && limit[0] > 0 && int64(len(parts)) > limit[0] {
		tail := strings.Join(parts[limit[0]-1:], separator)
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
			args[i] = uint64(phpval.Int(a) & 0xFFFFFFFF)
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
	// count returns the number of elements in $array; non-array values count as 0.
	rt.RegisterFunc("count", func(array any) int64 {
		n, _ := model.LenValues(array)
		return int64(n)
	})
	// in_array reports whether $needle occurs in $haystack, comparing values as strings; the $strict argument is accepted and ignored.
	rt.RegisterFunc("in_array", func(needle, haystack any, strict ...any) bool {
		found := false
		model.RangeValues(haystack, func(_, v any) bool {
			if phpval.String(v) == phpval.String(needle) {
				found = true
				return false
			}
			return true
		})
		return found
	})
	// array_unique returns $array with duplicate values removed, comparing values as strings and keeping the first occurrence and its key; the $flags argument is accepted and ignored.
	rt.RegisterFunc("array_unique", func(array any, flags ...any) *model.Array {
		n, _ := model.LenValues(array)
		out := model.NewArraySize(n)
		seen := make(map[string]bool, n)
		model.RangeValues(array, func(k, v any) bool {
			s := phpval.String(v)
			if !seen[s] {
				seen[s] = true
				out.Set(k, v)
			}
			return true
		})
		return out
	})
	// array_merge merges the given arrays into one; integer keys are renumbered and later string keys overwrite earlier ones.
	rt.RegisterFunc("array_merge", phpArrayMerge)
	// array_keys returns the keys of $array as a list; there is no $filter_value parameter.
	rt.RegisterFunc("array_keys", func(array any) []any {
		n, _ := model.LenValues(array)
		out := make([]any, 0, n)
		model.RangeValues(array, func(k, _ any) bool { out = append(out, k); return true })
		return out
	})
	// array_values returns the values of $array as a list indexed from zero.
	rt.RegisterFunc("array_values", func(array any) []any {
		n, _ := model.LenValues(array)
		out := make([]any, 0, n)
		model.RangeValues(array, func(_, v any) bool { out = append(out, v); return true })
		return out
	})
	// array_slice returns up to $length elements of $array starting at $offset, a negative $offset counting from the end; keys are discarded and reindexed from zero, and a negative $length yields an empty array.
	rt.RegisterFunc("array_slice", phpArraySlice)
	// array_splice removes $length elements of $array at $offset, inserts $replacement in their place, and returns the removed elements; a value that is not a script array is an error.
	rt.RegisterFunc("array_splice", phpArraySplice)
	// array_map returns a list of $callback applied to each value of $array; a single array is accepted and keys are not preserved.
	rt.RegisterFunc("array_map", func(callback any, array any) ([]any, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return nil, errors.New("array_map(): argument #1 ($callback) must be a valid callback")
		}
		return phpArrayMap(fn, array)
	})
	// usort sorts $array in place using the $callback comparator, reindexing the keys from zero.
	rt.RegisterFunc("usort", func(array any, callback any) (bool, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return false, errors.New("usort(): argument #2 ($callback) must be a valid callback")
		}
		return phpUsort(array, fn), nil
	})
	// sort sorts $array in place ascending with PHP's default comparison, discarding the keys and reindexing from zero.
	rt.RegisterFunc("sort", phpSort)
	// rsort sorts $array in place descending with PHP's default comparison, discarding the keys and reindexing from zero.
	rt.RegisterFunc("rsort", phpRsort)
}

// phpArrayMerge implements array_merge. PHP renumbers integer keys and lets
// later string keys win.
//
// This returns an *model.Array even when every input is a list. Returning a
// presized []any for that case is tempting, because it is the shape
// `call_user_func_array($fn, array_merge(...))` forwards with no copy, but
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
// genuinely requires a *model.Array. Passing anything else is an error rather
// than a silent no-op.
func phpArraySplice(array any, offset int64, optional ...any) ([]any, error) {
	a, ok := array.(*model.Array)
	if !ok || a == nil {
		return nil, fmt.Errorf("array_splice: expected an array, got %T", array)
	}

	vals := phpval.Values(a)
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
	l := int(phpval.Int(lengthArg))
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
		return phpval.Values(rep)
	}
	return []any{rep}
}

// phpArraySlice returns the selected run as a []any. PHP's array_slice
// reindexes integer keys, which this shim has always done for every key, so a
// list is a faithful representation of the result.
func phpArraySlice(array any, offset int64, length ...int64) []any {
	vals := phpval.Values(array)
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
		return phpval.Int(r) < 0
	})
}

// phpSort orders a list with PHP's default comparison (see phpval.Compare). Like
// sort(), it discards the keys and reindexes the result from zero.
func phpSort(array any) bool {
	if sortInt64Array(array, false) {
		return true
	}
	return sortValues(array, sortLess)
}

// phpRsort is phpSort in descending order.
func phpRsort(array any) bool {
	if sortInt64Array(array, true) {
		return true
	}
	return sortValues(array, func(x, y any) bool { return sortLess(y, x) })
}

func sortInt64Array(a any, desc bool) bool {
	arr, ok := a.(*model.Array)
	if !ok || arr == nil {
		return false
	}
	vals, ok := arr.Int64List()
	if !ok {
		return false
	}
	arrayi64.Sort(vals, len(vals))
	if desc {
		for i, j := 0, len(vals)-1; i < j; i, j = i+1, j-1 {
			vals[i], vals[j] = vals[j], vals[i]
		}
	}
	arr.ReplaceInt64List(vals)
	return true
}

// sortLess is the boolean sort.SliceStable wants, derived from phpval.Compare so
// that ties stay ties.
func sortLess(x, y any) bool {
	return phpval.Compare(x, y) < 0
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
		vals := phpval.Values(target)
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
	// json_encode returns the JSON encoding of $value; there is no $flags parameter and an encoding failure raises an error instead of returning false.
	rt.RegisterFunc("json_encode", phpJSONEncode)
	// json_decode parses the JSON in $text; objects always decode to arrays (as if $associative were true) and invalid input raises an error instead of returning null.
	rt.RegisterFunc("json_decode", phpJSONDecode)
}

func phpJSONEncode(value any) (any, error) {
	b, err := json.Marshal(jsonEncodeValue(value))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func phpJSONDecode(text string) (any, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(text))
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
			out[phpval.String(k)] = jsonEncodeValue(v)
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
	// spl_autoload_register registers $callback as an autoloader, or the default spl_autoload when $callback is null or omitted; $prepend puts it first and $throw is ignored.
	rt.RegisterFunc("spl_autoload_register", func(args ...any) (bool, error) {
		var callback any = rt.SPLAutoload
		if len(args) > 0 && args[0] != nil {
			callback = args[0]
		}
		prepend := len(args) > 2 && phpval.Truthy(args[2])
		rt.RegisterAutoloader(callback, prepend)
		return true, nil
	})
	// spl_autoload loads $class by including the lowercased class name plus ".php" from the include path; the $file_extensions argument is accepted and ignored.
	rt.RegisterFunc("spl_autoload", func(class string, fileExtensions ...any) error {
		return rt.SPLAutoload(class)
	})
	// class_exists reports whether class $class is defined, running the autoloader first unless $autoload is false.
	rt.RegisterFunc("class_exists", func(class string, autoload ...bool) (bool, error) {
		load := true
		if len(autoload) > 0 {
			load = autoload[0]
		}
		return rt.ClassExists(class, load)
	})
	rt.RegisterFunc("set_include_path", rt.SetIncludePath)
	rt.RegisterFunc("get_include_path", rt.IncludePath)
	// get_defined_constants returns the defined constants as a name-sorted array; with $categorize true they are grouped under a single "Core" key.
	rt.RegisterFunc("get_defined_constants", func(categorize ...bool) *model.Array {
		// The introspection shims keep returning *model.Array: their whole value
		// is a stable, name-sorted listing, which a Go map cannot express.
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
	// get_defined_functions returns an array with "internal" and "user" lists of function names; the $exclude_disabled argument is accepted and ignored.
	rt.RegisterFunc("get_defined_functions", func(excludeDisabled ...bool) map[string][]string {
		internal, user := rt.DefinedFunctions()
		return map[string][]string{"internal": internal, "user": user}
	})
	// get_defined_vars returns the variables defined in the calling scope as an array sorted by name, not in definition order.
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
	// get_declared_classes returns the names of the declared classes.
	rt.RegisterFunc("get_declared_classes", func() []string {
		return rt.DeclaredClasses()
	})
	// php_sapi_name returns the name of the SAPI the script runs under, such as "cli".
	rt.RegisterFunc("php_sapi_name", func() string {
		return rt.SAPI()
	})
	// isset reports whether every argument is set and not null.
	rt.RegisterFunc("isset", func(args ...any) bool {
		for _, a := range args {
			if a == nil {
				return false
			}
		}
		return true
	})
	// empty reports whether $value is empty: null, false, "", "0", 0, 0.0 or an empty array.
	rt.RegisterFunc("empty", func(value any) bool { return !phpval.Truthy(value) })
	// A binding's []string is as much a PHP array as an *model.Array is, so
	// is_array() answers for the whole value model, not one Go type.
	rt.RegisterFunc("is_array", model.IsCollection)
	// is_int reports whether $value is an integer.
	rt.RegisterFunc("is_int", func(value any) bool {
		switch value.(type) {
		case int, int64:
			return true
		}
		return false
	})
	// is_string reports whether $value is a string.
	rt.RegisterFunc("is_string", func(value any) bool { _, ok := value.(string); return ok })
	// is_bool reports whether $value is a boolean.
	rt.RegisterFunc("is_bool", func(value any) bool { _, ok := value.(bool); return ok })
	// is_object reports whether $value is an object. A value a Go binding
	// returned is one: a script constructs it with new, calls its methods and
	// reads its properties the same way it does an interpreted object.
	rt.RegisterFunc("is_object", isObject)
	// get_included_files returns the names of the files included or required so far.
	rt.RegisterFunc("get_included_files", func() []string { return rt.IncludedFiles() })
	// is_numeric reports whether $value is an int or a float; unlike PHP, numeric strings return false.
	rt.RegisterFunc("is_numeric", func(value any) bool {
		switch value.(type) {
		case int64, int, float64:
			return true
		}
		return false
	})
	// intdiv returns the integer quotient of $num divided by $divisor; division by zero and PHP_INT_MIN by -1 are errors.
	rt.RegisterFunc("intdiv", func(num, divisor int64) (int64, error) {
		if divisor == 0 {
			return 0, errors.New("Division by zero")
		}
		if num == math.MinInt64 && divisor == -1 {
			return 0, errors.New("Division of PHP_INT_MIN by -1 is not an integer")
		}
		return num / divisor, nil
	})
	// fdiv is IEEE-754 division: dividing by zero yields INF/-INF/NAN
	// instead of an error.
	rt.RegisterFunc("fdiv", func(num, divisor float64) float64 {
		return num / divisor
	})
	// call_user_func calls $callback with the given arguments and returns its result.
	rt.RegisterFunc("call_user_func", func(callback any, args ...any) (any, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return nil, errors.New("call_user_func(): argument #1 ($callback) must be a valid callback")
		}
		return fn(args...)
	})
	// call_user_func_array calls $callback with the values of $args as its arguments and returns its result.
	rt.RegisterFunc("call_user_func_array", func(callback any, args any) (any, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return nil, errors.New("call_user_func_array(): argument #1 ($callback) must be a valid callback")
		}
		return phpCallUserFuncArray(fn, args)
	})
	// function_exists reports whether a function named $function is defined.
	rt.RegisterFunc("function_exists", func(name string) bool { return rt.FunctionExists(name) })
	// is_callable reports whether $value can be called as a function; there are no $syntax_only or $callable_name parameters.
	rt.RegisterFunc("is_callable", func(value any) bool { _, ok := rt.Callable(value); return ok })
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
				status = int(phpval.Int(code[0]))
			}
		}
		return nil, rt.Exit(status)
	}
	// exit terminates the script; a string $status is printed before exiting with code 0, an int $status becomes the exit code.
	rt.RegisterFunc("exit", terminate)
	// die terminates the script exactly like exit; a string $status is printed before exiting with code 0, an int $status becomes the exit code.
	rt.RegisterFunc("die", terminate)
}

func phpCallUserFuncArray(fn func(...any) (any, error), args any) (any, error) {
	// A []any argument list is already the shape fn wants, so the common case
	// (array_merge's output, func_get_args) forwards without a copy.
	if callArgs, ok := args.([]any); ok {
		return fn(callArgs...)
	}
	return fn(phpval.Values(args)...)
}

// ---------------------------------------------------------------------------
// tokenizer + constants
// ---------------------------------------------------------------------------

func registerTokenizer(rt *runner.Runtime) {
	rt.RegisterFunc("token_get_all", parser.TokenGetAll)
	// token_name returns the name of token $id, such as "T_VARIABLE"; an unknown id returns "UNKNOWN".
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

// isObject reports whether value is what PHP calls an object, backing
// is_object.
//
// An interpreted object is one. So is a value a Go binding handed over: a
// script constructs it with new, calls its methods and reads its properties
// the same way, and get_class, method_exists and spl_object_id all reflect
// over it to answer.
//
// A collection is an array rather than an object, which is why the check comes
// before the struct test: *model.Array is itself a pointer to a struct.
func isObject(value any) bool {
	if value == nil {
		return false
	}
	if _, ok := value.(*model.Object); ok {
		return true
	}
	if model.IsCollection(value) {
		return false
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	return rv.Kind() == reflect.Struct
}
