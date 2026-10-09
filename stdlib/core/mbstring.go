package core

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/runner"
)

// The split between this file and strings.go is PHP's own: the str* functions
// count bytes and the mb_* functions count characters. A byte offset is what
// PREG_OFFSET_CAPTURE reports and what substr consumes, so the two agree;
// mb_substr takes a character offset, which a script derives with
// mb_strlen(substr($s, 0, $offset)).
//
// Every mb_* function here accepts and ignores a trailing $encoding, except
// where "8bit" selects byte units, which is PHP's own escape hatch.

// init contributes the mb_* functions to stdlib.Register.
func init() {
	runner.RegisterBinding(registerMultibyte)
}

func registerMultibyte(rt *runner.Runtime) {
	// mb_strlen returns the number of characters in $string; the $encoding argument "8bit" answers in bytes, the byte-count escape hatch for binary data, and every other encoding is read as UTF-8.
	rt.RegisterFunc("mb_strlen", func(s string, encoding ...any) int64 {
		if eightBit([]any(encoding), 0) {
			return int64(len(s))
		}
		return runeLen(s)
	})
	// mb_substr returns the part of $string selected by character offset $start and $length; the "8bit" encoding selects bytes, as substr does.
	rt.RegisterFunc("mb_substr", func(s string, start int64, rest ...any) string {
		if eightBit(rest, 1) {
			return phpSubstr(s, start, optionalLength(rest)...)
		}
		return mbSubstr(s, start, optionalLength(rest)...)
	})
	// mb_strpos returns the character offset of the first $needle in $haystack, or false; the "8bit" encoding reports byte offsets, as strpos does.
	rt.RegisterFunc("mb_strpos", func(haystack, needle string, rest ...any) any {
		if eightBit(rest, 1) {
			return phpStrpos(haystack, needle, optionalOffset(rest)...)
		}
		return mbStrpos(haystack, needle, optionalOffset(rest)...)
	})
	// mb_stripos returns the character offset of the first case-insensitive $needle in $haystack, or false; the fold is Unicode-wide, where stripos folds A-Z only.
	rt.RegisterFunc("mb_stripos", func(haystack, needle string, rest ...any) any {
		return mbStrpos(runeLower(haystack), runeLower(needle), optionalOffset(rest)...)
	})
	// mb_strrpos returns the character offset of the last $needle in $haystack, or false.
	rt.RegisterFunc("mb_strrpos", func(haystack, needle string, rest ...any) any {
		i := searchLast(haystack, needle, mbSearchLastOffset(haystack, rest))
		if i < 0 {
			return false
		}
		return byteRuneOffset(haystack, i)
	})
	// mb_str_split returns $string cut into chunks of $length characters; the "8bit" encoding cuts bytes, as str_split does.
	rt.RegisterFunc("mb_str_split", func(s string, rest ...any) []string {
		n := int64(1)
		if len(rest) > 0 {
			if l, ok := toOptionalInt(rest[0]); ok && l > 1 {
				n = l
			}
		}
		if eightBit(rest, 1) {
			return byteSplit(s, n)
		}
		return mbStrSplit(s, n)
	})
	// mb_str_pad returns $string padded to $length characters with $pad_string, the character-counting str_pad PHP 8.3 added.
	rt.RegisterFunc("mb_str_pad", func(s string, length int64, optional ...any) string {
		return mbStrPad(s, length, optional...)
	})
	// mb_substr_count returns the number of non-overlapping occurrences of $needle in $haystack, the whole string, $offset and $length not being arguments PHP's mb_ spelling takes.
	rt.RegisterFunc("mb_substr_count", func(haystack, needle string, _ ...any) int64 {
		if needle == "" {
			return 0
		}
		return int64(strings.Count(haystack, needle))
	})
	// mb_strtoupper returns $string uppercased, non-ASCII letters included, where strtoupper folds A-Z only.
	rt.RegisterFunc("mb_strtoupper", func(s string, _ ...any) string { return strings.ToUpper(s) })
	// mb_strtolower returns $string lowercased, non-ASCII letters included, where strtolower folds A-Z only.
	rt.RegisterFunc("mb_strtolower", func(s string, _ ...any) string { return strings.ToLower(s) })
	// mb_ucfirst returns $string with its first character uppercased, non-ASCII letters included.
	rt.RegisterFunc("mb_ucfirst", mbUcfirst)
	// mb_lcfirst returns $string with its first character lowercased, non-ASCII letters included.
	rt.RegisterFunc("mb_lcfirst", mbLcfirst)
}

// toOptionalInt reads an optional numeric argument a script may pass as int.
func toOptionalInt(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case float64:
		return int64(x), true
	}
	return 0, false
}

// optionalOffset adapts a mixed optional-argument tail ($offset, then the
// ignored $encoding) to the offset slice the search implementations take.
func optionalOffset(rest []any) []int64 {
	if len(rest) > 0 {
		if o, ok := toOptionalInt(rest[0]); ok {
			return []int64{o}
		}
	}
	return nil
}

// optionalLength is optionalOffset for the $length of a substring call, where
// an explicit null means "to the end" and is dropped, never read as 0.
func optionalLength(rest []any) []int64 {
	if len(rest) > 0 && rest[0] != nil {
		if l, ok := toOptionalInt(rest[0]); ok {
			return []int64{l}
		}
	}
	return nil
}

// mbSearchLastOffset converts mb_strrpos's character $offset into the byte
// offset searchLast takes, the two functions sharing one search.
func mbSearchLastOffset(haystack string, rest []any) []int64 {
	offset := optionalOffset(rest)
	if len(offset) == 0 {
		return nil
	}
	o := offset[0]
	if o < 0 {
		// A negative cap counts characters from the end, and the bytes of the
		// characters it skips are what searchLast caps on.
		tail, _ := runeByteOffset(haystack, runeLen(haystack)+o)
		return []int64{int64(tail) - int64(len(haystack))}
	}
	from, ok := runeByteOffset(haystack, o)
	if !ok {
		return []int64{int64(len(haystack)) + 1}
	}
	return []int64{int64(from)}
}

// mbSubstr is substr counting characters, PHP's mb_substr.
func mbSubstr(s string, start int64, length ...int64) string {
	n := runeLen(s)
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
		return ""
	}
	from, _ := runeByteOffset(s, start)
	to, _ := runeByteOffset(s, end)
	return s[from:to]
}

// mbStrpos is strpos counting characters, PHP's mb_strpos.
func mbStrpos(haystack, needle string, offset ...int64) any {
	start := int64(0)
	if len(offset) > 0 {
		start = offset[0]
		if start < 0 {
			start += runeLen(haystack)
			if start < 0 {
				start = 0
			}
		}
	}
	from, ok := runeByteOffset(haystack, start)
	if !ok {
		return false
	}
	i := strings.Index(haystack[from:], needle)
	if i < 0 {
		return false
	}
	return byteRuneOffset(haystack, i+from)
}

// mbStrSplit is str_split counting characters, PHP's mb_str_split.
func mbStrSplit(s string, n int64) []string {
	if s == "" {
		return []string{}
	}
	size := runeLen(s)
	out := make([]string, 0, (size+n-1)/n)
	for i := int64(0); i < size; i += n {
		end := i + n
		if end > size {
			end = size
		}
		from, _ := runeByteOffset(s, i)
		to, _ := runeByteOffset(s, end)
		out = append(out, s[from:to])
	}
	return out
}

// mbStrPad is str_pad counting characters, PHP's mb_str_pad.
func mbStrPad(s string, length int64, optional ...any) string {
	pad := " "
	if len(optional) > 0 {
		pad = phpval.String(optional[0])
	}
	padType := int64(strPadRight)
	if len(optional) > 1 {
		padType = phpval.Int(optional[1])
	}
	diff := length - runeLen(s)
	if diff <= 0 || pad == "" {
		return s
	}
	switch padType {
	case strPadLeft:
		return mbPadTo(pad, diff) + s
	case strPadBoth:
		left := diff / 2
		return mbPadTo(pad, left) + s + mbPadTo(pad, diff-left)
	default:
		return s + mbPadTo(pad, diff)
	}
}

// mbPadTo repeats pad to exactly n characters, cutting the last repetition
// short.
func mbPadTo(pad string, n int64) string {
	if n <= 0 {
		return ""
	}
	padLen := runeLen(pad)
	whole := strings.Repeat(pad, int(n/padLen)+1)
	cut, _ := runeByteOffset(whole, n)
	return whole[:cut]
}

// mbUcfirst uppercases the first character, non-ASCII letters included.
func mbUcfirst(str string) string {
	r, size := utf8.DecodeRuneInString(str)
	if r == utf8.RuneError {
		return str
	}
	up := unicode.ToUpper(r)
	if up == r {
		return str
	}
	return string(up) + str[size:]
}

// mbLcfirst is mbUcfirst in the other direction.
func mbLcfirst(str string) string {
	r, size := utf8.DecodeRuneInString(str)
	if r == utf8.RuneError {
		return str
	}
	low := unicode.ToLower(r)
	if low == r {
		return str
	}
	return string(low) + str[size:]
}

// runeLen is the character count of s.
func runeLen(s string) int64 { return int64(utf8.RuneCountInString(s)) }

// runeByteOffset converts a character offset into a byte offset in s. An
// offset past the last character answers len(s) and false.
func runeByteOffset(s string, runes int64) (int, bool) {
	if runes <= 0 {
		return 0, true
	}
	n := int64(0)
	for i := range s {
		if n == runes {
			return i, true
		}
		n++
	}
	if n == runes {
		return len(s), true
	}
	return len(s), false
}

// byteRuneOffset is the character count of s[:bytes], converting a byte
// position a search found back into the character offset a script sees.
func byteRuneOffset(s string, bytes int) int64 {
	return int64(utf8.RuneCountInString(s[:bytes]))
}

// runeLower folds s rune by rune. strings.ToLower with simple mappings keeps
// the rune count, so character offsets computed on the folded string index
// the original.
func runeLower(s string) string {
	return strings.Map(unicode.ToLower, s)
}

// eightBit reports the "8bit" encoding at position at of an optional-
// argument tail, the byte-unit selector PHP's mb_* functions honour.
func eightBit(rest []any, at int) bool {
	if len(rest) <= at {
		return false
	}
	enc, ok := rest[at].(string)
	return ok && strings.EqualFold(enc, "8bit")
}

// byteSplit is str_split in byte units, shared by str_split and
// mb_str_split's "8bit" encoding.
func byteSplit(s string, n int64) []string {
	if s == "" {
		return []string{}
	}
	size := int64(len(s))
	out := make([]string, 0, (size+n-1)/n)
	for i := int64(0); i < size; i += n {
		end := i + n
		if end > size {
			end = size
		}
		out = append(out, s[i:end])
	}
	return out
}
