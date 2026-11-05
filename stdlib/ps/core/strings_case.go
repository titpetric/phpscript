package core

import (
	"strings"
)

// asciiLower folds A-Z only, as PHP's strtolower does and all its
// case-insensitive search functions fold by. strings.ToLower is Unicode-aware
// and can change a string's byte length, which would shift the offsets those
// searches report.
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

// asciiUpper is asciiLower in the other direction, PHP's strtoupper.
func asciiUpper(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'a' && c <= 'z' {
			b := []byte(s)
			for ; i < len(b); i++ {
				if c := b[i]; c >= 'a' && c <= 'z' {
					b[i] = c - 'a' + 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

// ucwordsDefaultSeparators is PHP's default $separators for ucwords.
const ucwordsDefaultSeparators = " \t\r\n\f\v"

// phpUcfirst maps the first byte up when that byte is a-z and leaves every
// other first byte alone and ucfirst("ábc") returns "ábc". mbUcfirst converts
// the first character, ASCII or not.
func phpUcfirst(str string) string {
	if str == "" || str[0] < 'a' || str[0] > 'z' {
		return str
	}
	return string(str[0]-'a'+'A') + str[1:]
}

// phpLcfirst is phpUcfirst in the other direction.
func phpLcfirst(str string) string {
	if str == "" || str[0] < 'A' || str[0] > 'Z' {
		return str
	}
	return string(str[0]-'A'+'a') + str[1:]
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
			// The copy starts at the first byte that changes and phpUcwords
			// returns a string already in the wanted case as it arrived.
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
