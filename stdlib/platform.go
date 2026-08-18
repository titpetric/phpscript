package stdlib

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/model"
	phprunner "github.com/titpetric/phpscript/runner"
)

// This file provides the platform surface a PHP program written for stock PHP
// expects to find before it does anything interesting: the PHP_* constants, the
// reflection-ish helpers (get_class, method_exists), the ini/filter stubs, and
// the SPL exception hierarchy.
//
// composer's generated autoloader is the reason it exists in this shape. It
// gates on PHP_VERSION_ID, reports through PHP_SAPI/STDERR/PHP_EOL, probes for
// APCu with function_exists + ini_get + filter_var, and builds its class map
// with strtr/strrpos. None of that is composer-specific, and all of it is used by any
// non-trivial PHP library uses.

// phpVersion is the PHP language version phpscript reports. It is not a claim
// of full compatibility: it is the version whose semantics the runtime is
// tested against (see docs/changelog), and the number libraries branch on when
// they decide which language features they may use.
const (
	phpVersionMajor   = 8
	phpVersionMinor   = 4
	phpVersionRelease = 0
	phpVersion        = "8.4.0"
	phpVersionID      = phpVersionMajor*10000 + phpVersionMinor*100 + phpVersionRelease
)

func registerPlatform(rt *phprunner.Runtime) {
	registerPlatformConstants(rt)
	registerPlatformFuncs(rt)
	registerReflection(rt)
	registerExceptions(rt)
	registerClosure(rt)
}

func registerPlatformConstants(rt *phprunner.Runtime) {
	rt.SetConst("PHP_VERSION", phpVersion)
	rt.SetConst("PHP_MAJOR_VERSION", int64(phpVersionMajor))
	rt.SetConst("PHP_MINOR_VERSION", int64(phpVersionMinor))
	rt.SetConst("PHP_RELEASE_VERSION", int64(phpVersionRelease))
	rt.SetConst("PHP_EXTRA_VERSION", "")
	rt.SetConst("PHP_VERSION_ID", int64(phpVersionID))
	rt.SetConst("PHP_SAPI", rt.SAPI())
	rt.SetConst("PHP_EOL", "\n")
	rt.SetConst("PHP_OS", osName())
	rt.SetConst("PHP_OS_FAMILY", osFamily())
	rt.SetConst("PHP_INT_MAX", int64(math.MaxInt64))
	rt.SetConst("PHP_INT_MIN", int64(math.MinInt64))
	rt.SetConst("PHP_INT_SIZE", int64(8))
	rt.SetConst("PHP_FLOAT_EPSILON", math.Nextafter(1, 2)-1)
	rt.SetConst("PHP_FLOAT_MAX", math.MaxFloat64)
	rt.SetConst("PHP_FLOAT_MIN", math.SmallestNonzeroFloat64)
	rt.SetConst("PHP_FLOAT_DIG", int64(15))

	// The standard streams. STDIN is registered with the rest of the language
	// constructs; the two output streams are the process's, not the response
	// body, so a diagnostic written to STDERR stays out of the page.
	rt.SetConst("STDOUT", os.Stdout)
	rt.SetConst("STDERR", os.Stderr)

	// htmlspecialchars() flags. phpscript always escapes as ENT_QUOTES does, so
	// the values exist to be passed, not to select a behaviour.
	rt.SetConst("ENT_QUOTES", int64(3))
	rt.SetConst("ENT_COMPAT", int64(2))
	rt.SetConst("ENT_NOQUOTES", int64(0))
	rt.SetConst("ENT_HTML5", int64(48))
	rt.SetConst("ENT_SUBSTITUTE", int64(8))

	// filter_var() filters, of which only the validators below are implemented.
	rt.SetConst("FILTER_DEFAULT", int64(516))
	rt.SetConst("FILTER_VALIDATE_INT", int64(257))
	rt.SetConst("FILTER_VALIDATE_BOOLEAN", int64(258))
	rt.SetConst("FILTER_VALIDATE_BOOL", int64(258))
	rt.SetConst("FILTER_VALIDATE_FLOAT", int64(259))
	rt.SetConst("FILTER_NULL_ON_FAILURE", int64(134217728))

	rt.SetConst("E_ALL", int64(32767))
	rt.SetConst("E_ERROR", int64(1))
	rt.SetConst("E_WARNING", int64(2))
	rt.SetConst("E_NOTICE", int64(8))
	rt.SetConst("E_DEPRECATED", int64(8192))
	rt.SetConst("E_STRICT", int64(2048))
}

// osName reports the PHP_OS value for the host. PHP uses the uname system
// name, which for the platforms Go builds on differs from GOOS only in case.
func osName() string {
	switch runtime.GOOS {
	case "windows":
		return "WINNT"
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	default:
		return strings.ToUpper(runtime.GOOS[:1]) + runtime.GOOS[1:]
	}
}

// osFamily reports PHP_OS_FAMILY: the coarse grouping PHP code branches on.
func osFamily() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	case "dragonfly", "freebsd", "netbsd", "openbsd":
		return "BSD"
	case "aix", "solaris", "illumos":
		return "Solaris"
	default:
		return "Unknown"
	}
}

func registerPlatformFuncs(rt *phprunner.Runtime) {
	rt.RegisterFunc("phpversion", func(_ ...any) any { return phpVersion })
	rt.RegisterFunc("php_uname", func(_ ...any) string { return osName() })
	rt.RegisterFunc("zend_version", func() string { return phpVersion })

	rt.RegisterFunc("define", func(name string, value any, _ ...any) bool {
		rt.SetConst(name, value)
		return true
	})
	rt.RegisterFunc("defined", func(name string) bool {
		_, ok := rt.Const(name)
		return ok
	})
	rt.RegisterFunc("constant", func(name string) (any, error) {
		if value, ok := rt.Const(name); ok {
			return value, nil
		}
		return nil, fmt.Errorf("undefined constant %q", name)
	})

	// Response headers are staged and written after the script finishes (see
	// runner.Context.Header), so nothing has been sent while PHP is running.
	rt.RegisterFunc("headers_sent", func(_ ...any) bool { return false })

	// phpscript has no php.ini. Reporting every directive as unset is what PHP
	// itself does for an unknown one, and it is the answer library code treats
	// as "this extension is not configured".
	rt.RegisterFunc("ini_get", func(_ string) any { return false })
	rt.RegisterFunc("ini_set", func(_ string, _ any) any { return false })
	rt.RegisterFunc("error_reporting", func(_ ...any) int64 { return 0 })
	rt.RegisterFunc("set_error_handler", func(_ ...any) any { return nil })
	rt.RegisterFunc("extension_loaded", func(_ string) bool { return false })

	rt.RegisterFunc("filter_var", phpFilterVar)

	rt.RegisterFunc("strtr", phpStrtr)
	rt.RegisterFunc("strrpos", func(haystack, needle string, offset ...int64) any {
		start := 0
		if len(offset) > 0 && offset[0] > 0 {
			start = int(offset[0])
			if start > len(haystack) {
				return false
			}
		}
		i := strings.LastIndex(haystack[start:], needle)
		if i < 0 {
			return false
		}
		return int64(i + start)
	})

	rt.RegisterFunc("spl_autoload_unregister", func(callback any) bool {
		return rt.UnregisterAutoloader(callback)
	})
	rt.RegisterFunc("stream_resolve_include_path", func(name string) any {
		if filepath.IsAbs(name) {
			if _, err := os.Stat(name); err == nil {
				return name
			}
			return false
		}
		for _, dir := range filepath.SplitList(rt.IncludePath()) {
			if dir == "" {
				dir = "."
			}
			candidate := path.Join(filepath.ToSlash(dir), name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		return false
	})
}

// phpFilterVar implements the filter_var validators phpscript supports. An
// unknown filter passes the value through, which is what FILTER_DEFAULT does.
func phpFilterVar(value any, args ...any) any {
	filter := int64(516)
	if len(args) > 0 {
		filter = toInt64(args[0])
	}
	text := strings.TrimSpace(toString(value))
	switch filter {
	case 257: // FILTER_VALIDATE_INT
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return false
		}
		return n
	case 258: // FILTER_VALIDATE_BOOLEAN
		switch strings.ToLower(text) {
		case "1", "true", "on", "yes":
			return true
		case "0", "false", "off", "no", "":
			return false
		}
		return false
	case 259: // FILTER_VALIDATE_FLOAT
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return false
		}
		return f
	default:
		return value
	}
}

// phpStrtr implements both spellings: strtr($s, $from, $to) replaces bytes
// positionally, strtr($s, $pairs) replaces substrings, longest match first.
func phpStrtr(subject string, args ...any) string {
	if len(args) == 0 {
		return subject
	}
	if len(args) >= 2 {
		from, to := toString(args[0]), toString(args[1])
		n := min(len(from), len(to))
		if n == 0 {
			return subject
		}
		out := []byte(subject)
		for i := range out {
			if j := strings.IndexByte(from[:n], out[i]); j >= 0 {
				out[i] = to[j]
			}
		}
		return string(out)
	}
	pairs, ok := args[0].(*model.Array)
	if !ok {
		return subject
	}
	// PHP tries the longest key that matches at each position, so the
	// replacement is order-independent and never rewrites its own output.
	keys := make([]string, 0, pairs.Len())
	replacements := make(map[string]string, pairs.Len())
	pairs.Range(func(key, val any) bool {
		text := toString(key)
		if text == "" {
			return true
		}
		keys = append(keys, text)
		replacements[text] = toString(val)
		return true
	})
	var out strings.Builder
	out.Grow(len(subject))
	for i := 0; i < len(subject); {
		best := ""
		for _, key := range keys {
			if len(key) > len(best) && strings.HasPrefix(subject[i:], key) {
				best = key
			}
		}
		if best == "" {
			out.WriteByte(subject[i])
			i++
			continue
		}
		out.WriteString(replacements[best])
		i += len(best)
	}
	return out.String()
}

func registerReflection(rt *phprunner.Runtime) {
	rt.RegisterFunc("get_class", func(args ...any) any {
		if len(args) == 0 || args[0] == nil {
			return false
		}
		return classNameOf(args[0])
	})
	rt.RegisterFunc("get_parent_class", func(_ ...any) any { return false })
	rt.RegisterFunc("get_object_vars", func(value any) *model.Array {
		out := model.NewArray()
		if object, ok := value.(*model.Object); ok {
			for _, field := range object.Class.Fields {
				if v, ok := object.Props[field.Name]; ok {
					out.Set(field.Name, v)
				}
			}
			for name, v := range object.Props {
				if _, seen := out.Get(name); !seen {
					out.Set(name, v)
				}
			}
		}
		return out
	})
	rt.RegisterFunc("method_exists", func(target any, method string) bool {
		if name, ok := target.(string); ok {
			return rt.MethodExists(name, method)
		}
		if object, ok := target.(*model.Object); ok && object.Class != nil {
			return rt.MethodExists(object.Class.Name, method)
		}
		value := reflect.ValueOf(target)
		if !value.IsValid() {
			return false
		}
		for i := 0; i < value.NumMethod(); i++ {
			if strings.EqualFold(value.Type().Method(i).Name, method) {
				return true
			}
		}
		return false
	})
	rt.RegisterFunc("property_exists", func(target any, property string) bool {
		object, ok := target.(*model.Object)
		if !ok {
			return false
		}
		if _, ok := object.Props[property]; ok {
			return true
		}
		if object.Class == nil {
			return false
		}
		for _, field := range object.Class.Fields {
			if field.Name == property {
				return true
			}
		}
		return false
	})
	// PHP hands out an opaque, per-object identity. A pointer is exactly that,
	// and it is stable for as long as the object is alive, which is the only
	// guarantee PHP makes either.
	rt.RegisterFunc("spl_object_hash", func(value any) string {
		return fmt.Sprintf("%032x", objectIdentity(value))
	})
	rt.RegisterFunc("spl_object_id", func(value any) int64 {
		return int64(objectIdentity(value))
	})
}

// classNameOf reports the PHP class name of a value: the declared name for an
// interpreted object, and the Go type name for a host-backed one.
func classNameOf(value any) string {
	if object, ok := value.(*model.Object); ok && object.Class != nil {
		return object.Class.Name
	}
	t := reflect.TypeOf(value)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return ""
	}
	return t.Name()
}

// objectIdentity returns a stable numeric identity for an object value.
func objectIdentity(value any) uintptr {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return 0
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.UnsafePointer, reflect.Func:
		return rv.Pointer()
	}
	return 0
}

// splExceptions are the SPL and Error class names a PHP library throws. None of
// them adds behaviour over Exception, and phpscript has no exception hierarchy
// to filter a catch on, so they all construct the same value, which is what makes
// `throw new \InvalidArgumentException(...)` work rather than fail on an
// undefined class.
var splExceptions = []string{
	"ErrorException",
	"RuntimeException",
	"LogicException",
	"InvalidArgumentException",
	"DomainException",
	"LengthException",
	"OutOfRangeException",
	"OutOfBoundsException",
	"RangeException",
	"OverflowException",
	"UnderflowException",
	"UnexpectedValueException",
	"BadFunctionCallException",
	"BadMethodCallException",
	"JsonException",
	"Error",
	"TypeError",
	"ValueError",
	"ArithmeticError",
	"DivisionByZeroError",
	"ArgumentCountError",
}

func registerExceptions(rt *phprunner.Runtime) {
	for _, name := range splExceptions {
		rt.RegisterConstructor(name, NewException)
	}
}

func registerClosure(rt *phprunner.Runtime) {
	// Closure::bind rebinds a closure's scope. phpscript enforces no property
	// visibility, so a scope change has nothing to alter and the closure is
	// returned as it is. Rebinding `$this` would change what the body sees and
	// is therefore refused rather than silently ignored.
	rt.RegisterFunc("Closure::bind", func(closure any, args ...any) (any, error) {
		if len(args) > 0 && args[0] != nil {
			return nil, fmt.Errorf("Closure::bind(): rebinding $this is not supported")
		}
		if _, ok := rt.Callable(closure); !ok {
			return nil, fmt.Errorf("Closure::bind(): argument #1 ($closure) must be a closure")
		}
		return closure, nil
	})
	rt.RegisterFunc("Closure::fromCallable", func(callback any) (any, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return nil, fmt.Errorf("Closure::fromCallable(): argument #1 ($callback) is not callable")
		}
		return fn, nil
	})
}

// toInt64 coerces a filter argument to its integer form.
func toInt64(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	case bool:
		if x {
			return 1
		}
		return 0
	}
	return 0
}
