package core

import (
	"hash/crc32"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the string shims to stdlib.Register.
func init() {
	runner.RegisterBinding(registerStrings)
}

func registerStrings(rt *runner.Runtime) {
	// strlen returns the length of $str in bytes; mb_strlen counts characters.
	rt.RegisterFunc("strlen", func(str string) int64 { return int64(len(str)) })
	// strtoupper returns $string with A-Z mapped up and every other byte left
	// alone; mb_strtoupper converts non-ASCII letters too.
	rt.RegisterFunc("strtoupper", asciiUpper)
	// strtolower returns $string with A-Z mapped down and every other byte
	// left alone; mb_strtolower converts non-ASCII letters too.
	rt.RegisterFunc("strtolower", asciiLower)
	// trim strips whitespace, or the bytes listed in $characters, from both
	// ends of $string; "a..z" in the list is the range between the two.
	rt.RegisterFunc("trim", phpTrim(trimBoth, defaultTrimChars))
	// rtrim strips whitespace, or the bytes listed in $characters, from the
	// end of $string; "a..z" in the list is the range between the two.
	rt.RegisterFunc("rtrim", phpTrim(trimRight, defaultTrimChars))
	// ltrim strips whitespace, or the bytes listed in $characters, from the
	// start of $string; "a..z" in the list is the range between the two.
	rt.RegisterFunc("ltrim", phpTrim(trimLeft, defaultTrimChars))

	// substr returns the part of $string from byte $offset for $length bytes;
	// a negative $offset counts from the end and a negative $length stops
	// that many bytes before it. mb_substr counts characters.
	rt.RegisterFunc("substr", phpSubstr)
	// strpos returns the byte offset of the first $needle in $haystack, or
	// false when it does not occur; a negative $offset counts from the end of
	// $haystack.
	rt.RegisterFunc("strpos", phpStrpos)
	// stripos returns the byte offset of the first case-insensitive $needle
	// in $haystack, or false when it does not occur; a negative $offset
	// counts from the end of $haystack.
	rt.RegisterFunc("stripos", phpStripos)
	// strrpos returns the byte offset of the last $needle in $haystack, or
	// false when it does not occur.
	rt.RegisterFunc("strrpos", phpStrrpos)
	// strripos returns the byte offset of the last case-insensitive $needle
	// in $haystack, or false when it does not occur; a negative $offset
	// requires the match to start that many bytes before the end.
	rt.RegisterFunc("strripos", phpStrripos)
	// substr_count returns the number of non-overlapping occurrences of
	// $needle in $haystack, restricted to the byte window $offset and $length
	// describe.
	rt.RegisterFunc("substr_count", phpSubstrCount)
	// substr_replace returns $string with the bytes from $offset for $length
	// replaced by $replace; a negative $offset counts from the end and a
	// negative $length is a distance from it. Array arguments are not
	// supported.
	rt.RegisterFunc("substr_replace", phpSubstrReplace)
	// ucfirst returns $string with a leading a-z mapped up and every other
	// first byte left alone; mb_ucfirst converts non-ASCII letters too.
	rt.RegisterFunc("ucfirst", phpUcfirst)
	// lcfirst returns $string with a leading A-Z mapped down and every other
	// first byte left alone; mb_lcfirst converts non-ASCII letters too.
	rt.RegisterFunc("lcfirst", phpLcfirst)
	// ucwords returns $string with the first ASCII letter of every word
	// uppercased, where $separators separates the words and defaults to "
	// \t\r\n\f\v".
	rt.RegisterFunc("ucwords", phpUcwords)
	// chr returns the one-byte string for $codepoint, taken modulo 256 with
	// negative values wrapping up into that range, as PHP does.
	rt.RegisterFunc("chr", phpChr)
	// ord returns the first byte of $character as an integer, or 0 when $character is empty.
	rt.RegisterFunc("ord", phpOrd)
	// str_contains reports whether $needle occurs in $haystack; every string
	// contains an empty needle.
	rt.RegisterFunc("str_contains", phpStrContains)
	// str_starts_with reports whether $haystack begins with $needle.
	rt.RegisterFunc("str_starts_with", phpStrStartsWith)
	// str_ends_with reports whether $haystack ends with $needle.
	rt.RegisterFunc("str_ends_with", phpStrEndsWith)
	// strrev returns $string with its bytes in reverse order, so a multi-byte
	// character comes out as its bytes in reverse.
	rt.RegisterFunc("strrev", phpStrrev)
	// str_split returns $string cut into chunks of $length bytes, the last one
	// shorter when the string does not divide evenly, so a chunk boundary may
	// fall inside a multi-byte character. An empty string returns an empty
	// array.
	rt.RegisterFunc("str_split", phpStrSplit)
	rt.SetConst("FNM_PATHNAME", int64(fnmPathname))
	rt.SetConst("FNM_NOESCAPE", int64(fnmNoescape))
	rt.SetConst("FNM_PERIOD", int64(fnmPeriod))
	rt.SetConst("FNM_CASEFOLD", int64(fnmCasefold))
	// fnmatch reports whether $str matches the shell wildcard $pattern, where
	// * and ? cross a directory separator unless $flags names FNM_PATHNAME;
	// the other flags are FNM_NOESCAPE, FNM_PERIOD and FNM_CASEFOLD.
	rt.RegisterFunc("fnmatch", phpFnmatch)
	rt.SetConst("STR_PAD_RIGHT", int64(strPadRight))
	rt.SetConst("STR_PAD_LEFT", int64(strPadLeft))
	rt.SetConst("STR_PAD_BOTH", int64(strPadBoth))
	// str_pad returns $string padded with $pad_string to $length characters
	// on the side $pad_type selects; a $length below the current one is a
	// no-op.
	rt.RegisterFunc("str_pad", phpStrPad)
	// strstr returns $haystack from the first occurrence of $needle to the
	// end, or false when it does not occur; there is no $before_needle
	// parameter.
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
	// implode returns the values of $array joined with $separator; with a
	// single array argument the separator is "".
	rt.RegisterFunc("implode", phpImplode)
	// join returns the values of $array joined with $separator, PHP's alias of implode.
	rt.RegisterFunc("join", phpImplode)
	// explode splits $str on $separator into a list; a positive $limit caps
	// the parts, the last one holding the rest, and other limits do nothing.
	rt.RegisterFunc("explode", phpExplode)
	// htmlspecialchars escapes &, <, >, double and single quotes as HTML
	// entities; it takes the $flags and later arguments and ignores them.
	rt.RegisterFunc("htmlspecialchars", func(s string, flags ...any) string {
		return htmlSpecialCharsReplacer.Replace(s)
	})
	// sprintf returns $format with each conversion replaced by the argument
	// it names, coerced the way PHP renders a value in a string context;
	// width, precision and padding count bytes.
	rt.RegisterFunc("sprintf", phpSprintf)
	// crc32 returns the CRC-32 checksum of $str as an integer.
	rt.RegisterFunc("crc32", phpCRC32)
}

// htmlSpecialCharsReplacer holds one Replacer for the whole package: a
// strings.Replacer compiles a trie over its pairs, and htmlspecialchars runs
// once per template variable, so constructing it inside the binding paid for
// that trie on every call.
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
// crc32 keys and etags are short, run the table loop in place with no
// allocation. Longer ones keep the standard library's implementation, which
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
// it the loop beats allocating a copy (8 bytes: 38ns/0 allocs against
// 105ns/1 alloc); at 256 the two are level on this hardware.
// See BenchmarkCRC32.
const crc32NativeThreshold = 256

// phpSubstr implements substr($s, $start[, $length]) with PHP's negative
// offset and length semantics, counting bytes. mbSubstr counts characters.
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

// chrTable holds the 256 one-byte strings chr can return. Building it once
// means chr allocates nothing per call, where string(byte(n)) would cost an
// allocation for every byte outside the runtime's small-string cache.
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
