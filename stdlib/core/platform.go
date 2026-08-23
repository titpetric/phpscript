package core

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the platform constants and functions to stdlib.Register.
func init() {
	runner.RegisterBinding(registerPlatform)
}

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

func registerPlatform(rt *runner.Runtime) {
	registerPlatformConstants(rt)
	registerPlatformFuncs(rt)
}

func registerPlatformConstants(rt *runner.Runtime) {
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

	// $_FILES error codes. A script compares against these whether or not the
	// runtime can produce them, so all of PHP's are defined; the request
	// runtime sets OK, INI_SIZE, NO_FILE, NO_TMP_DIR and CANT_WRITE, since
	// FORM_SIZE is a per-form limit, PARTIAL is an aborted transfer, and
	// EXTENSION comes from extensions phpscript has no equivalent of.
	rt.SetConst("UPLOAD_ERR_OK", int64(runner.UploadErrOK))
	rt.SetConst("UPLOAD_ERR_INI_SIZE", int64(runner.UploadErrIniSize))
	rt.SetConst("UPLOAD_ERR_FORM_SIZE", int64(2))
	rt.SetConst("UPLOAD_ERR_PARTIAL", int64(3))
	rt.SetConst("UPLOAD_ERR_NO_FILE", int64(runner.UploadErrNoFile))
	rt.SetConst("UPLOAD_ERR_NO_TMP_DIR", int64(runner.UploadErrNoTmpDir))
	rt.SetConst("UPLOAD_ERR_CANT_WRITE", int64(runner.UploadErrCantWrite))
	rt.SetConst("UPLOAD_ERR_EXTENSION", int64(8))

	// E_ALL is not every bit set: PHP 8 dropped E_STRICT (2048) from it, so the
	// value is 30719 rather than 32767. The difference is only visible once a
	// script can write E_ALL & ~E_NOTICE, which needs the bitwise operators.
	rt.SetConst("E_ALL", int64(30719))
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

func registerPlatformFuncs(rt *runner.Runtime) {
	// phpversion returns "8.4.0", the PHP language version phpscript reports; an $extension argument is ignored.
	rt.RegisterFunc("phpversion", func(extension ...any) any { return phpVersion })
	// php_uname returns the host operating system name, the same value as PHP_OS; a $mode argument is ignored.
	rt.RegisterFunc("php_uname", func(mode ...any) string { return osName() })
	// zend_version returns the same version string as phpversion(); phpscript has no separate engine version.
	rt.RegisterFunc("zend_version", func() string { return phpVersion })

	// define defines constant $constant_name with value $value and returns true; the case-insensitivity flag is ignored.
	rt.RegisterFunc("define", func(constantName string, value any, _ ...any) bool {
		rt.SetConst(constantName, value)
		return true
	})
	// defined reports whether constant $constant_name is defined.
	rt.RegisterFunc("defined", func(constantName string) bool {
		_, ok := rt.Const(constantName)
		return ok
	})
	// constant returns the value of constant $name; an undefined name is an error.
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
	// ini_set accepts and ignores $option and $value and returns false; phpscript has no php.ini to change.
	rt.RegisterFunc("ini_set", func(option string, value any) any { return false })
	// error_reporting always returns 0 and ignores $error_level; the reporting level is not configurable.
	rt.RegisterFunc("error_reporting", func(errorLevel ...any) int64 { return 0 })
	// set_error_handler accepts and ignores $callback and returns null; the handler is never invoked.
	rt.RegisterFunc("set_error_handler", func(callback ...any) any { return nil })
	// extension_loaded returns false for every $extension name; phpscript loads no extensions.
	rt.RegisterFunc("extension_loaded", func(extension string) bool { return false })

	rt.RegisterFunc("filter_var", phpFilterVar)

	rt.RegisterFunc("strtr", phpStrtr)
	// strrpos returns the byte position of the last occurrence of $needle in $haystack, or false if it does not occur; a positive $offset skips that many leading bytes and a negative one requires the match to start that many bytes before the end.
	rt.RegisterFunc("strrpos", func(haystack, needle string, offset ...int64) any {
		if len(offset) == 0 || offset[0] == 0 {
			return lastIndexOrFalse(strings.LastIndex(haystack, needle))
		}
		if o := offset[0]; o > 0 {
			if o > int64(len(haystack)) {
				return false
			}
			i := strings.LastIndex(haystack[o:], needle)
			if i < 0 {
				return false
			}
			return int64(i) + o
		}
		// A negative offset caps where the match may start, counted from the
		// end. PHP 8 raises a ValueError when that runs past the start of the
		// haystack; phpscript clamps to 0. A match inside the prefix ending
		// one needle past the cap starts at or before it, so LastIndex over
		// that prefix answers directly.
		last := int64(len(haystack)) + offset[0]
		if last < 0 {
			last = 0
		}
		end := last + int64(len(needle))
		if end > int64(len(haystack)) {
			end = int64(len(haystack))
		}
		return lastIndexOrFalse(strings.LastIndex(haystack[:end], needle))
	})

	// spl_autoload_unregister removes autoloader $callback and reports whether it was registered.
	rt.RegisterFunc("spl_autoload_unregister", func(callback any) bool {
		return rt.UnregisterAutoloader(callback)
	})
	// stream_resolve_include_path resolves $filename against the include path and returns the first path that exists, or false when none does.
	rt.RegisterFunc("stream_resolve_include_path", func(filename string) any {
		if filepath.IsAbs(filename) {
			if _, err := os.Stat(filename); err == nil {
				return filename
			}
			return false
		}
		for _, dir := range filepath.SplitList(rt.IncludePath()) {
			if dir == "" {
				dir = "."
			}
			candidate := path.Join(filepath.ToSlash(dir), filename)
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
	text := strings.TrimSpace(phpval.String(value))
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

// lastIndexOrFalse maps a strings.LastIndex result onto the int|false union
// strrpos returns, since PHP callers compare the miss with === false.
func lastIndexOrFalse(i int) any {
	if i < 0 {
		return false
	}
	return int64(i)
}

// phpStrtr implements both spellings: strtr($subject, $from, $to) replaces
// bytes positionally, strtr($subject, $pairs) replaces substrings, longest
// match first.
func phpStrtr(subject string, args ...any) string {
	if len(args) == 0 {
		return subject
	}
	if len(args) >= 2 {
		from, to := phpval.String(args[0]), phpval.String(args[1])
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
		text := phpval.String(key)
		if text == "" {
			return true
		}
		keys = append(keys, text)
		replacements[text] = phpval.String(val)
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
