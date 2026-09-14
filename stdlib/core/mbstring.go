package core

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/titpetric/phpscript/runner"
)

// The str* functions count in characters, not bytes: strlen("Čedo") is 4,
// substr slices code points, and the strpos family reports and accepts
// character offsets. PHP's own str* count bytes and reserve character
// semantics for mb_*; phpscript makes the character behaviour the default
// (docs/README.md, known divergences) and registers the mb_* names as
// aliases of the same implementations, so code written against either
// family reads UTF-8 correctly. The byte-level functions - ord, chr,
// bin2hex, strcmp, trim - keep byte semantics: they are the binary API.

// init contributes the mb_* aliases to stdlib.Register.
func init() {
	runner.RegisterBinding(registerMultibyte)
}

func registerMultibyte(rt *runner.Runtime) {
	// mb_strlen returns the number of characters in $string; the $encoding argument "8bit" answers in bytes, the byte-count escape hatch for binary data, and every other encoding is read as UTF-8.
	rt.RegisterFunc("mb_strlen", func(s string, encoding ...any) int64 {
		if len(encoding) > 0 {
			if enc, ok := encoding[0].(string); ok && strings.EqualFold(enc, "8bit") {
				return int64(len(s))
			}
		}
		return int64(utf8.RuneCountInString(s))
	})
	// mb_substr returns the part of $string selected by character offset $start and $length; the "8bit" encoding selects bytes, as PHP's does.
	rt.RegisterFunc("mb_substr", func(s string, start int64, rest ...any) string {
		if eightBit(rest, 1) {
			return byteSubstr(s, start, rest)
		}
		if len(rest) > 0 && rest[0] != nil {
			if l, ok := toOptionalInt(rest[0]); ok {
				return phpSubstr(s, start, l)
			}
		}
		return phpSubstr(s, start)
	})
	// mb_strpos returns the character offset of the first $needle in $haystack, or false; the "8bit" encoding reports byte offsets, as PHP's does.
	rt.RegisterFunc("mb_strpos", func(haystack, needle string, rest ...any) any {
		if eightBit(rest, 1) {
			start := int64(0)
			if len(rest) > 0 {
				if o, ok := toOptionalInt(rest[0]); ok {
					start = o
				}
			}
			if start < 0 {
				start += int64(len(haystack))
			}
			if start < 0 || start > int64(len(haystack)) {
				return false
			}
			i := strings.Index(haystack[start:], needle)
			if i < 0 {
				return false
			}
			return int64(i) + start
		}
		return phpStrpos(haystack, needle, optionalOffset(rest)...)
	})
	// mb_stripos returns the character offset of the first case-insensitive $needle in $haystack, or false.
	rt.RegisterFunc("mb_stripos", func(haystack, needle string, rest ...any) any {
		return phpStripos(haystack, needle, optionalOffset(rest)...)
	})
	// mb_strrpos returns the character offset of the last $needle in $haystack, or false.
	rt.RegisterFunc("mb_strrpos", func(haystack, needle string, rest ...any) any {
		return phpStrrpos(haystack, needle, optionalOffset(rest)...)
	})
	// mb_str_split returns $string cut into chunks of $length characters; the "8bit" encoding cuts bytes, as PHP's does.
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
		return phpStrSplit(s, n)
	})
	// mb_strtoupper returns $string uppercased.
	rt.RegisterFunc("mb_strtoupper", func(s string, _ ...any) string { return strings.ToUpper(s) })
	// mb_strtolower returns $string lowercased.
	rt.RegisterFunc("mb_strtolower", func(s string, _ ...any) string { return strings.ToLower(s) })
	// mb_ucfirst returns $string with its first character uppercased.
	rt.RegisterFunc("mb_ucfirst", phpUcfirst)
	// mb_lcfirst returns $string with its first character lowercased.
	rt.RegisterFunc("mb_lcfirst", phpLcfirst)
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
// ignored $encoding) to the offset slice the str* implementations take.
func optionalOffset(rest []any) []int64 {
	if len(rest) > 0 {
		if o, ok := toOptionalInt(rest[0]); ok {
			return []int64{o}
		}
	}
	return nil
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

// byteSubstr is substr in byte units, PHP's own substr semantics, kept for
// mb_substr's "8bit" encoding.
func byteSubstr(s string, start int64, rest []any) string {
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
	if len(rest) > 0 && rest[0] != nil {
		if l, ok := toOptionalInt(rest[0]); ok {
			if l < 0 {
				end = n + l
			} else {
				end = start + l
			}
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

// byteSplit is str_split in byte units for mb_str_split's "8bit" encoding.
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
