package core

import (
	"fmt"
	"hash/crc32"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the string shims to stdlib.Register.
func init() {
	runner.RegisterBinding(registerStrings)
}

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
