package core

import (
	"strings"
)

// asciiLower folds A-Z only, as strtolower does; byte length never changes.
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

// phpUcfirst uppercases an a-z first byte and leaves any other first byte alone.
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

// phpUcwords uppercases the first ASCII letter of every word, byte-wise.
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
