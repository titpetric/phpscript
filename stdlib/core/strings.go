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
	// strpos returns the byte offset of the first $needle in $haystack, or false when it does not occur; a negative $offset counts from the end of $haystack.
	rt.RegisterFunc("strpos", phpStrpos)
	// stripos returns the byte offset of the first case-insensitive $needle in $haystack, or false when it does not occur; a negative $offset counts from the end of $haystack.
	rt.RegisterFunc("stripos", phpStripos)
	// strripos returns the byte offset of the last case-insensitive $needle in $haystack, or false when it does not occur; a negative $offset requires the match to start that many bytes before the end.
	rt.RegisterFunc("strripos", phpStrripos)
	// substr_count returns the number of non-overlapping occurrences of $needle in $haystack, restricted to the window $offset and $length describe.
	rt.RegisterFunc("substr_count", phpSubstrCount)
	// substr_replace returns $string with the bytes from $offset for $length replaced by $replace; a negative $offset counts from the end and a negative $length is a distance from it. Array arguments are not supported.
	rt.RegisterFunc("substr_replace", phpSubstrReplace)
	// ucfirst returns $string with its first byte uppercased if it is an ASCII letter; like PHP's non-mb functions it never changes the byte length.
	rt.RegisterFunc("ucfirst", phpUcfirst)
	// lcfirst returns $string with its first byte lowercased if it is an ASCII letter; like PHP's non-mb functions it never changes the byte length.
	rt.RegisterFunc("lcfirst", phpLcfirst)
	// ucwords returns $string with the first ASCII letter of every word uppercased, words being separated by $separators, which defaults to " \t\r\n\f\v".
	rt.RegisterFunc("ucwords", phpUcwords)
	// chr returns the one-byte string for $codepoint, taken modulo 256 with negative values wrapping up into that range, as PHP does.
	rt.RegisterFunc("chr", phpChr)
	// ord returns the first byte of $character as an integer, or 0 when $character is empty.
	rt.RegisterFunc("ord", phpOrd)
	// str_contains reports whether $needle occurs in $haystack; an empty needle is contained in every string.
	rt.RegisterFunc("str_contains", phpStrContains)
	// str_starts_with reports whether $haystack begins with $needle.
	rt.RegisterFunc("str_starts_with", phpStrStartsWith)
	// str_ends_with reports whether $haystack ends with $needle.
	rt.RegisterFunc("str_ends_with", phpStrEndsWith)
	// strrev returns $string with its bytes in reverse order; multi-byte characters are not preserved, matching PHP.
	rt.RegisterFunc("strrev", phpStrrev)
	// str_split returns $string cut into chunks of $length bytes, the last one shorter when the string does not divide evenly; an empty string yields an empty array.
	rt.RegisterFunc("str_split", phpStrSplit)
	rt.SetConst("STR_PAD_RIGHT", int64(strPadRight))
	rt.SetConst("STR_PAD_LEFT", int64(strPadLeft))
	rt.SetConst("STR_PAD_BOTH", int64(strPadBoth))
	// str_pad returns $string padded with $pad_string to $length bytes on the side $pad_type selects; a $length below the current one is a no-op.
	rt.RegisterFunc("str_pad", phpStrPad)
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
	// join returns the values of $array joined with $separator, PHP's alias of implode.
	rt.RegisterFunc("join", phpImplode)
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

// STR_PAD_* selectors, as PHP numbers them.
const (
	strPadLeft  = 0
	strPadRight = 1
	strPadBoth  = 2
)

// phpStrpos implements strpos($haystack, $needle[, $offset]).
func phpStrpos(haystack, needle string, offset ...int64) any {
	start, ok := searchStart(len(haystack), offset)
	if !ok {
		return false
	}
	i := strings.Index(haystack[start:], needle)
	if i < 0 {
		return false
	}
	return int64(i + start)
}

// phpStripos implements stripos($haystack, $needle[, $offset]). Both operands
// are folded so the returned offset still indexes the original haystack.
func phpStripos(haystack, needle string, offset ...int64) any {
	return phpStrpos(asciiLower(haystack), asciiLower(needle), offset...)
}

// phpStrripos implements strripos($haystack, $needle[, $offset]).
func phpStrripos(haystack, needle string, offset ...int64) any {
	i := searchLast(asciiLower(haystack), asciiLower(needle), offset)
	if i < 0 {
		return false
	}
	return int64(i)
}

// searchStart resolves the strpos-family $offset against a haystack of n
// bytes. A negative offset counts from the end; PHP 8 raises a ValueError when
// it lands before the start, which phpscript clamps to 0 instead. The bool is
// false when the offset is past the end, where the search cannot match.
func searchStart(n int, offset []int64) (int, bool) {
	if len(offset) == 0 {
		return 0, true
	}
	start := offset[0]
	if start < 0 {
		start += int64(n)
		if start < 0 {
			start = 0
		}
	}
	if start > int64(n) {
		return 0, false
	}
	return int(start), true
}

// searchLast is the strrpos-family search: the last match at or before the
// window $offset describes, or -1. A positive offset skips that many leading
// bytes, a negative one caps where the match may start, counted from the end.
// PHP 8 raises a ValueError when either runs off the haystack; phpscript
// clamps a negative overrun to the start and reports no match past the end.
func searchLast(haystack, needle string, offset []int64) int {
	if len(offset) == 0 || offset[0] == 0 {
		return strings.LastIndex(haystack, needle)
	}
	if o := offset[0]; o > 0 {
		if o > int64(len(haystack)) {
			return -1
		}
		i := strings.LastIndex(haystack[o:], needle)
		if i < 0 {
			return -1
		}
		return i + int(o)
	}
	last := int64(len(haystack)) + offset[0]
	if last < 0 {
		last = 0
	}
	// A match inside the prefix that ends one needle past the cap starts at
	// or before it, so LastIndex over that prefix answers directly.
	end := last + int64(len(needle))
	if end > int64(len(haystack)) {
		end = int64(len(haystack))
	}
	return strings.LastIndex(haystack[:end], needle)
}

// asciiLower folds A-Z only. strings.ToLower is Unicode-aware and can change a
// string's byte length, which would shift the offsets the case-insensitive
// search functions report; PHP folds the ASCII range and nothing else.
func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for ; i < len(b); i++ {
				if c := b[i]; c >= 'A' && c <= 'Z' {
					b[i] = c + 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

// ucwordsDefaultSeparators is PHP's default $separators for ucwords.
const ucwordsDefaultSeparators = " \t\r\n\f\v"

// phpUcfirst uppercases the leading byte. strings.ToUpper is Unicode-aware and
// would change the byte length of a multi-byte first character; PHP's ucfirst
// touches the ASCII range and nothing else, so a non-ASCII leading byte is
// returned untouched.
func phpUcfirst(str string) string {
	if str == "" || str[0] < 'a' || str[0] > 'z' {
		return str
	}
	b := []byte(str)
	b[0] -= 'a' - 'A'
	return string(b)
}

// phpLcfirst is phpUcfirst in the other direction, ASCII-only for the same reason.
func phpLcfirst(str string) string {
	if str == "" || str[0] < 'A' || str[0] > 'Z' {
		return str
	}
	b := []byte(str)
	b[0] += 'a' - 'A'
	return string(b)
}

// phpUcwords uppercases the first ASCII letter of every word. A word starts at
// the beginning of the string and after any byte listed in $separators. The
// scan is byte-wise for the reason phpUcfirst is: PHP's ucwords does not fold
// non-ASCII letters and must not change the byte length.
func phpUcwords(str string, separators ...string) string {
	seps := ucwordsDefaultSeparators
	if len(separators) > 0 {
		seps = separators[0]
	}
	var b []byte
	start := true
	for i := 0; i < len(str); i++ {
		c := str[i]
		if start && c >= 'a' && c <= 'z' {
			// The copy is made on the first byte that actually changes, so a
			// string already in the wanted case is returned as it arrived.
			if b == nil {
				b = []byte(str)
			}
			b[i] = c - ('a' - 'A')
		}
		start = strings.IndexByte(seps, c) >= 0
	}
	if b == nil {
		return str
	}
	return string(b)
}

// chrTable holds the 256 one-byte strings chr can return. Building it once
// means chr allocates nothing per call, where string(byte(n)) would allocate
// for every byte outside the runtime's small-string cache.
var chrTable = func() [256]string {
	var t [256]string
	for i := range t {
		t[i] = string([]byte{byte(i)})
	}
	return t
}()

// phpChr returns the byte $codepoint names. PHP reduces the argument modulo
// 256 and wraps negatives up into 0-255, so chr(256) is chr(0) and chr(-1) is
// chr(255).
func phpChr(codepoint int64) string {
	c := codepoint % 256
	if c < 0 {
		c += 256
	}
	return chrTable[c]
}

// phpOrd returns the first byte, not the first rune: PHP's ord is byte-wise, so
// a multi-byte character reports its leading byte. An empty string is 0.
func phpOrd(character string) int64 {
	if character == "" {
		return 0
	}
	return int64(character[0])
}

// The three PHP 8 substring predicates are one-line wrappers so the published
// signature names $haystack and $needle rather than the standard library's
// positional parameters.
func phpStrContains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

func phpStrStartsWith(haystack, needle string) bool { return strings.HasPrefix(haystack, needle) }

func phpStrEndsWith(haystack, needle string) bool { return strings.HasSuffix(haystack, needle) }

// phpStrrev reverses bytes. PHP's strrev is byte-wise too, so a multi-byte
// character comes out as its bytes in reverse.
func phpStrrev(str string) string {
	b := []byte(str)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

// phpStrSplit returns the chunks as a []string for the reason phpExplode does:
// the loop allocates exactly that slice and the VM indexes it like any array.
func phpStrSplit(str string, length ...int64) []string {
	// PHP 8.2 returns an empty array for an empty subject; older versions
	// returned a single empty chunk.
	if str == "" {
		return []string{}
	}
	n := int64(1)
	// PHP 8 raises a ValueError below 1; phpscript clamps.
	if len(length) > 0 && length[0] > 1 {
		n = length[0]
	}
	size := int64(len(str))
	out := make([]string, 0, (size+n-1)/n)
	for i := int64(0); i < size; i += n {
		end := i + n
		if end > size {
			end = size
		}
		out = append(out, str[i:end])
	}
	return out
}

// phpStrPad implements str_pad($string, $length, $pad_string, $pad_type). Go
// allows one variadic, so both optional arguments arrive together: the pad
// string first, the STR_PAD_* selector second.
func phpStrPad(str string, length int64, optional ...any) string {
	pad := " "
	if len(optional) > 0 {
		pad = phpval.String(optional[0])
	}
	padType := int64(strPadRight)
	if len(optional) > 1 {
		padType = phpval.Int(optional[1])
	}
	diff := length - int64(len(str))
	if diff <= 0 || pad == "" {
		return str
	}
	switch padType {
	case strPadLeft:
		return padTo(pad, int(diff)) + str
	case strPadBoth:
		// An odd remainder goes to the right, as PHP halves downwards.
		left := diff / 2
		return padTo(pad, int(left)) + str + padTo(pad, int(diff-left))
	default:
		return str + padTo(pad, int(diff))
	}
}

// padTo repeats pad to exactly n bytes, cutting the last repetition short.
func padTo(pad string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(pad, n/len(pad)+1)[:n]
}

// phpSubstrCount implements substr_count($haystack, $needle[, $offset[, $length]]).
// strings.Count is non-overlapping, which is what PHP counts.
func phpSubstrCount(haystack, needle string, optional ...any) int64 {
	// PHP 8 raises a ValueError on an empty needle.
	if needle == "" {
		return 0
	}
	return int64(strings.Count(substrWindow(haystack, optional), needle))
}

// substrWindow cuts the haystack down to the optional $offset and $length. A
// negative offset counts from the end and a negative length stops that many
// bytes before it. PHP 8 raises a ValueError when either leaves the string;
// phpscript clamps.
func substrWindow(haystack string, optional []any) string {
	n := int64(len(haystack))
	start := int64(0)
	if len(optional) > 0 {
		start = phpval.Int(optional[0])
		if start < 0 {
			start += n
		}
	}
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}
	end := n
	// An explicit null length means "to the end", same as omitting it.
	if len(optional) > 1 && optional[1] != nil {
		if l := phpval.Int(optional[1]); l < 0 {
			end = n + l
		} else {
			end = start + l
		}
	}
	if end > n {
		end = n
	}
	if end < start {
		end = start
	}
	return haystack[start:end]
}

// phpSubstrReplace implements substr_replace($string, $replace, $offset[, $length]).
// PHP clamps an out-of-range offset here rather than raising, so this does too.
func phpSubstrReplace(str, replace string, offset int64, length ...int64) string {
	n := int64(len(str))
	start := offset
	if start < 0 {
		start += n
		if start < 0 {
			start = 0
		}
	}
	if start > n {
		start = n
	}
	end := n
	if len(length) > 0 {
		// A negative length is a distance from the end of the string, not
		// a count of bytes to drop.
		if l := length[0]; l < 0 {
			end = n + l
		} else {
			end = start + l
		}
	}
	if end > n {
		end = n
	}
	if end < start {
		end = start
	}
	return str[:start] + replace + str[end:]
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
