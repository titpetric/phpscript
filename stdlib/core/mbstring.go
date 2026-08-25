package core

import (
	"strings"
	"unicode/utf8"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the mbstring shims to stdlib.Register.
func init() {
	runner.RegisterBinding(registerMbstring)
}

// registerMbstring binds the rune-aware counterparts of the byte-wise functions
// in strings.go. Every offset and length these take counts characters, not
// bytes, and the only encoding they implement is UTF-8: the optional $encoding
// argument PHP accepts is taken and ignored.
func registerMbstring(rt *runner.Runtime) {
	// mb_strtolower returns $string lowercased by Unicode rules; the $encoding argument is accepted and ignored, as only UTF-8 is implemented.
	rt.RegisterFunc("mb_strtolower", func(str string, encoding ...any) string { return strings.ToLower(str) })
	// mb_strtoupper returns $string uppercased by Unicode rules; the $encoding argument is accepted and ignored, as only UTF-8 is implemented.
	rt.RegisterFunc("mb_strtoupper", func(str string, encoding ...any) string { return strings.ToUpper(str) })
	// mb_strlen returns the length of $string in characters rather than bytes; the $encoding argument is accepted and ignored, as only UTF-8 is implemented.
	rt.RegisterFunc("mb_strlen", func(str string, encoding ...any) int64 {
		return int64(utf8.RuneCountInString(str))
	})
	// mb_substr returns the part of $string from character $start for $length characters, where a negative $start counts from the end and a negative $length stops that many characters before it; the $encoding argument is accepted and ignored.
	rt.RegisterFunc("mb_substr", phpMbSubstr)
}

// phpMbSubstr implements mb_substr($string, $start[, $length[, $encoding]]).
// Go allows one variadic, so both optional arguments arrive together: the
// length first, the encoding second. PHP clamps an out-of-range window here
// rather than raising, so this does too.
func phpMbSubstr(str string, start int64, optional ...any) string {
	n := int64(utf8.RuneCountInString(str))
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
	// An explicit null length means "to the end", same as omitting it.
	if len(optional) > 0 && optional[0] != nil {
		// A negative length is a distance from the end of the string, not a
		// count of characters to keep.
		if l := phpval.Int(optional[0]); l < 0 {
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
	return runeSlice(str, start, end)
}

// runeSlice returns the characters of s in [start, end) as a subslice of s.
// Walking the string for the two byte offsets keeps the result sharing s's
// backing array; a []rune round trip would allocate the runes and then the
// result string, for a substring the caller usually just prints.
func runeSlice(s string, start, end int64) string {
	if start >= end {
		return ""
	}
	i := int64(0)
	from := 0
	for off := range s {
		if i == start {
			from = off
		}
		if i == end {
			return s[from:off]
		}
		i++
	}
	return s[from:]
}
