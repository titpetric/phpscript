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
	"os"
	"sort"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"

	"github.com/titpetric/phpscript/stdlib/ps"
)

// Register installs the pure (non-filesystem) shims and PHP constants. Use
// RegisterFS to add filesystem IO bound to a root directory.
func Register(rt *runner.Runtime) {
	rt.RegisterConstructor("Exception", NewException)

	registerStrings(rt)
	registerArrays(rt)
	registerJSON(rt)
	registerRegex(rt)
	registerLang(rt)
	registerTokenizer(rt)

	ps.Register(rt)
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
		r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#039;")
		return r.Replace(s)
	})
	rt.RegisterFunc("sprintf", phpSprintf)
	rt.RegisterFunc("crc32", func(s string) int64 { return int64(crc32.ChecksumIEEE([]byte(s))) })
}

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

// phpStrReplace implements str_replace where search may be a string or array
// (with a scalar or array replacement), and subject is a string.
func phpStrReplace(search, replace, subject any) string {
	out := toString(subject)
	if sa, ok := search.(*model.Array); ok {
		ra, raIsArr := replace.(*model.Array)
		repl := arrStrings(ra)
		for i, s := range arrStrings(sa) {
			r := ""
			if raIsArr {
				if i < len(repl) {
					r = repl[i]
				}
			} else {
				r = toString(replace)
			}
			out = strings.ReplaceAll(out, s, r)
		}
		return out
	}
	return strings.ReplaceAll(out, toString(search), toString(replace))
}

func phpImplode(a, b any) string {
	// implode($glue, $array) or implode($array).
	glue := ""
	var arr *model.Array
	if x, ok := a.(*model.Array); ok {
		arr = x
	} else {
		glue = toString(a)
		arr, _ = b.(*model.Array)
	}
	return strings.Join(arrStrings(arr), glue)
}

func phpExplode(delim, s string, limit ...int64) *model.Array {
	parts := strings.Split(s, delim)
	if len(limit) > 0 && limit[0] > 0 && int64(len(parts)) > limit[0] {
		head := parts[:limit[0]-1]
		tail := strings.Join(parts[limit[0]-1:], delim)
		parts = append(append([]string{}, head...), tail)
	}
	out := model.NewArray()
	for _, p := range parts {
		out.Append(p)
	}
	return out
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

func registerArrays(rt *runner.Runtime) {
	rt.RegisterFunc("compact", phpCompact)
	rt.RegisterFunc("count", func(a any) int64 {
		if arr, ok := a.(*model.Array); ok {
			return int64(arr.Len())
		}
		return 0
	})
	rt.RegisterFunc("in_array", func(needle any, haystack *model.Array, _ ...any) bool {
		found := false
		haystack.Range(func(_, v any) bool {
			if toString(v) == toString(needle) {
				found = true
				return false
			}
			return true
		})
		return found
	})
	rt.RegisterFunc("array_unique", func(a *model.Array, _ ...any) *model.Array {
		out := model.NewArray()
		seen := map[string]bool{}
		a.Range(func(k, v any) bool {
			s := toString(v)
			if !seen[s] {
				seen[s] = true
				out.Set(k, v)
			}
			return true
		})
		return out
	})
	rt.RegisterFunc("array_merge", func(arrs ...*model.Array) *model.Array {
		out := model.NewArray()
		for _, a := range arrs {
			if a == nil {
				continue
			}
			a.Range(func(k, v any) bool {
				if _, isInt := k.(int64); isInt {
					out.Append(v)
				} else {
					out.Set(k, v)
				}
				return true
			})
		}
		return out
	})
	rt.RegisterFunc("array_keys", func(a *model.Array) *model.Array {
		out := model.NewArray()
		for _, k := range a.Keys() {
			out.Append(k)
		}
		return out
	})
	rt.RegisterFunc("array_values", func(a *model.Array) *model.Array {
		out := model.NewArray()
		a.Range(func(_, v any) bool { out.Append(v); return true })
		return out
	})
	rt.RegisterFunc("array_slice", phpArraySlice)
	rt.RegisterFunc("array_splice", phpArraySplice)
	rt.RegisterFunc("array_map", phpArrayMap)
	rt.RegisterFunc("usort", phpUsort)
}

// phpCompact builds an associative map from variables in the calling scope.
// Names that do not exist are omitted, matching PHP's compact().
func phpCompact(ctx context.Context, names ...string) map[string]any {
	out := map[string]any{}
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

func phpArraySplice(a *model.Array, offset int64, optional ...any) *model.Array {
	removed := model.NewArray()
	if a == nil {
		return removed
	}

	var vals []any
	a.Range(func(_, v any) bool {
		vals = append(vals, v)
		return true
	})

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

	for _, v := range vals[start:end] {
		removed.Append(v)
	}

	out := append([]any{}, vals[:start]...)
	out = append(out, replacement...)
	out = append(out, vals[end:]...)

	a.Clear()
	for _, v := range out {
		a.Append(v)
	}

	return removed
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
	if arr, ok := rep.(*model.Array); ok {
		var out []any
		arr.Range(func(_, v any) bool {
			out = append(out, v)
			return true
		})
		return out
	}
	return []any{rep}
}

func phpArraySlice(a *model.Array, offset int64, length ...int64) *model.Array {
	out := model.NewArray()
	if a == nil {
		return out
	}
	var vals []any
	a.Range(func(_, v any) bool { vals = append(vals, v); return true })
	start := int(offset)
	if start < 0 {
		start += len(vals)
	}
	if start < 0 {
		start = 0
	}
	if start > len(vals) {
		return out
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
	for _, v := range vals[start:end] {
		out.Append(v)
	}
	return out
}

func phpArrayMap(fn func(...any) (any, error), a *model.Array) (*model.Array, error) {
	out := model.NewArray()
	if a == nil {
		return out, nil
	}
	a.Range(func(_, v any) bool {
		mapped, err := fn(v)
		if err != nil {
			return false
		}
		out.Append(mapped)
		return true
	})
	return out, nil
}

// phpUsort sorts the array's values in place using cmp, reindexing with integer
// keys (PHP semantics). cmp is the env-adapted comparator: func(...any)(any,error).
func phpUsort(a *model.Array, cmp func(...any) (any, error)) bool {
	if a == nil {
		return false
	}
	var vals []any
	a.Range(func(_, v any) bool { vals = append(vals, v); return true })
	sort.SliceStable(vals, func(i, j int) bool {
		r, err := cmp(vals[i], vals[j])
		if err != nil {
			return false
		}
		return toInt(r) < 0
	})
	// Rebuild a as a 0-indexed list in place.
	*a = *model.NewArray()
	for _, v := range vals {
		a.Append(v)
	}
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

func jsonEncodeValue(v any) any {
	switch x := v.(type) {
	case *model.Array:
		if x == nil {
			return nil
		}
		if jsonArrayIsList(x) {
			out := make([]any, 0, x.Len())
			x.Range(func(_, v any) bool {
				out = append(out, jsonEncodeValue(v))
				return true
			})
			return out
		}
		out := map[string]any{}
		x.Range(func(k, v any) bool {
			out[toString(k)] = jsonEncodeValue(v)
			return true
		})
		return out
	case *model.Object:
		if x == nil {
			return nil
		}
		out := map[string]any{}
		for k, v := range x.Props {
			out[k] = jsonEncodeValue(v)
		}
		return out
	default:
		return v
	}
}

func jsonArrayIsList(a *model.Array) bool {
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
		out := model.NewArray()
		for _, v := range x {
			out.Append(jsonDecodeValue(v))
		}
		return out
	case map[string]any:
		out := model.NewArray()
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
	rt.RegisterFunc("get_defined_constants", func(categorize ...bool) *model.Array {
		constants := model.NewArray()
		defined := rt.DefinedConstants()
		names := make([]string, 0, len(defined))
		for name := range defined {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			constants.Set(name, defined[name])
		}
		if len(categorize) > 0 && categorize[0] {
			grouped := model.NewArray()
			grouped.Set("Core", constants)
			return grouped
		}
		return constants
	})
	rt.RegisterFunc("get_defined_functions", func(_ ...bool) *model.Array {
		internal, user := rt.DefinedFunctions()
		functions := model.NewArray()
		functions.Set("internal", stringArray(internal))
		functions.Set("user", stringArray(user))
		return functions
	})
	rt.RegisterFunc("get_defined_vars", func(ctx context.Context) *model.Array {
		vars := model.NewArray()
		scope, ok := runner.ScopeFromContext(ctx)
		if !ok {
			return vars
		}
		defined := scope.DefinedVars()
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
	rt.RegisterFunc("get_declared_classes", func() *model.Array {
		return stringArray(rt.DeclaredClasses())
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
	rt.RegisterFunc("is_array", func(a any) bool { _, ok := a.(*model.Array); return ok })
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
	rt.RegisterFunc("exit", func(code ...any) (any, error) {
		status := 0
		if len(code) > 0 {
			status = int(toInt(code[0]))
		}
		return nil, rt.Exit(status)
	})
	rt.RegisterFunc("die", func(code ...any) (any, error) {
		status := 0
		if len(code) > 0 {
			status = int(toInt(code[0]))
		}
		return nil, rt.Exit(status)
	})
}

func stringArray(values []string) *model.Array {
	result := model.NewArray()
	for _, value := range values {
		result.Append(value)
	}
	return result
}

func phpCallUserFuncArray(fn func(...any) (any, error), args *model.Array) (any, error) {
	var callArgs []any
	if args != nil {
		args.Range(func(_, v any) bool {
			callArgs = append(callArgs, v)
			return true
		})
	}
	return fn(callArgs...)
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
		return fmt.Sprintf("%d", x)
	case int:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%v", x)
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
		var n int64
		fmt.Sscanf(x, "%d", &n)
		return n
	default:
		return 0
	}
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
		return true
	}
}

// arrStrings returns the array's values as strings in order.
func arrStrings(a *model.Array) []string {
	if a == nil {
		return nil
	}
	var out []string
	a.Range(func(_, v any) bool { out = append(out, toString(v)); return true })
	return out
}
