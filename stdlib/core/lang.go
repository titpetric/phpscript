package core

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"sort"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes PHP's language constructs to stdlib.Register.
func init() {
	runner.RegisterBinding(registerLang)
}

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

// phpGetType backs gettype. The names it returns are PHP's original ones, and
// that is the whole of the function: "integer" and "double" where the value
// model and get_debug_type say int and float, "boolean" rather than bool, and
// NULL alone in capitals. Answering with the modern spellings would pass every
// eyeball and fail every `gettype($x) === "integer"`.
func phpGetType(value any) string {
	switch value.(type) {
	case nil:
		return "NULL"
	case bool:
		return "boolean"
	case int, int64:
		return "integer"
	case float64:
		return "double"
	case string:
		return "string"
	}
	if model.IsCollection(value) {
		return "array"
	}
	if isObject(value) {
		return "object"
	}
	// A closure reaches a binding as a Go func rather than an instance, but
	// PHP has it an instance of Closure, so it reports as an object.
	if reflect.ValueOf(value).Kind() == reflect.Func {
		return "object"
	}
	return "unknown type"
}

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
	// gettype returns the type of $value under PHP's legacy names: "integer", "double", "boolean", "string", "array", "object" or "NULL".
	rt.RegisterFunc("gettype", phpGetType)
	// is_string reports whether $value is a string.
	rt.RegisterFunc("is_string", func(value any) bool { _, ok := value.(string); return ok })
	// is_bool reports whether $value is a boolean.
	rt.RegisterFunc("is_bool", func(value any) bool { _, ok := value.(bool); return ok })
	// is_null reports whether $value is null.
	rt.RegisterFunc("is_null", func(value any) bool { return value == nil })
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
	// intval returns the integer value of $value; $base applies only to a string $value, read like C strtol with the 0x and 0b prefixes, and a $base outside 0 and 2-36 yields 0.
	rt.RegisterFunc("intval", phpIntval)
	// intdiv returns the integer quotient of $num divided by $divisor; division by zero and PHP_INT_MIN by -1 are errors.
	rt.RegisterFunc("intdiv", func(num, divisor int64) (int64, error) {
		if divisor == 0 {
			return 0, &runner.DivisionByZeroError{Message: "Division by zero"}
		}
		if num == math.MinInt64 && divisor == -1 {
			return 0, &runner.ArithmeticError{Message: "Division of PHP_INT_MIN by -1 is not an integer"}
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

// phpIntval backs intval. Without a base, or with the default base 10, it is
// the (int) cast, which phpval.Int already is for every scalar; a collection is
// its truthiness (intval([]) is 0, intval([1, 2]) is 1), which the cast owns
// here because phpval.Int reads a collection as 0. Any other base applies to a
// string argument only — intval(12.9, 16) is 12 — and reads it through
// strtolInt.
func phpIntval(num any, base ...any) int64 {
	radix := int64(10)
	if len(base) > 0 {
		radix = phpval.Int(base[0])
	}
	if str, ok := num.(string); ok && radix != 10 {
		return strtolInt(str, radix)
	}
	if model.IsCollection(num) {
		if n, _ := model.LenValues(num); n > 0 {
			return 1
		}
		return 0
	}
	return phpval.Int(num)
}

// strtolInt reads s the way intval hands a non-10 base to C strtol: leading
// whitespace, one optional sign, then digits of the base until the first
// character that is not one, saturating at the int64 bounds the way strtol
// does. Base 0 detects the base from a 0x, 0b or 0 prefix and is 10 without
// one; 0b is PHP's own addition for bases 0 and 2, stripped before strtol ever
// sees the string. A base outside 0 and 2-36 is strtol's EINVAL, which intval
// surfaces as 0 rather than an error — php 8.5 raises no ValueError. The 0o
// octal prefix is not recognised, exactly because strtol does not know it:
// intval("0o12", 8) is 0.
func strtolInt(s string, base int64) int64 {
	if base != 0 && (base < 2 || base > 36) {
		return 0
	}
	i := 0
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			i++
			continue
		}
		break
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	// strtol takes a 0x prefix only when a hex digit follows; without one it
	// reads the lone "0" and stops. Both spellings of that are 0, so the
	// prefix is taken unconditionally here.
	switch {
	case (base == 0 || base == 16) && i+1 < len(s) && s[i] == '0' && (s[i+1] == 'x' || s[i+1] == 'X'):
		base, i = 16, i+2
	case (base == 0 || base == 2) && i+1 < len(s) && s[i] == '0' && (s[i+1] == 'b' || s[i+1] == 'B'):
		base, i = 2, i+2
	case base == 0:
		base = 10
		if i < len(s) && s[i] == '0' {
			base = 8
		}
	}
	// The magnitude accumulates unsigned so that -9223372036854775808 base 10
	// still reads exactly; one digit past the bound saturates like strtol.
	var mag uint64
	overflowed := false
	for ; i < len(s); i++ {
		var d uint64
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			d = uint64(c - '0')
		case c >= 'a' && c <= 'z':
			d = uint64(c-'a') + 10
		case c >= 'A' && c <= 'Z':
			d = uint64(c-'A') + 10
		default:
			d = 36
		}
		if d >= uint64(base) {
			break
		}
		if overflowed {
			continue
		}
		if mag > (math.MaxUint64-d)/uint64(base) {
			overflowed = true
			continue
		}
		mag = mag*uint64(base) + d
	}
	if neg {
		if overflowed || mag > 1<<63 {
			return math.MinInt64
		}
		return -int64(mag)
	}
	if overflowed || mag > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(mag)
}
