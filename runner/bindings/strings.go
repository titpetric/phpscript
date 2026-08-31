package bindings

import (
	"github.com/titpetric/phpscript/runner"
)

// init contributes the string comparisons to stdlib.Register.
func init() {
	runner.RegisterBinding(registerStrings)
}

func registerStrings(rt *runner.Runtime) {
	// strcmp compares $string1 and $string2 byte by byte, answering a negative number when $string1 sorts first, a positive one when it sorts last, and 0 when they are equal.
	rt.RegisterFunc("strcmp", func(string1, string2 string) int64 {
		return compareBytes(string1, string2, wholeString, false)
	})
	// strcasecmp compares $string1 and $string2 byte by byte with the ASCII letters folded to lower case, answering a negative number, a positive one, or 0 as strcmp does; a byte above 127 is compared as it is, so the folding does not reach an accented letter.
	rt.RegisterFunc("strcasecmp", func(string1, string2 string) int64 {
		return compareBytes(string1, string2, wholeString, true)
	})
	// strncmp compares at most $length leading bytes of $string1 and $string2 the way strcmp does; a $length past the end of both compares what is there, and a negative one compares nothing and answers 0.
	rt.RegisterFunc("strncmp", func(string1, string2 string, length int64) int64 {
		return compareBytes(string1, string2, clampLength(length), false)
	})
	// strncasecmp compares at most $length leading bytes of $string1 and $string2 the way strcasecmp does, with the same reading of a $length past the end or below zero.
	rt.RegisterFunc("strncasecmp", func(string1, string2 string, length int64) int64 {
		return compareBytes(string1, string2, clampLength(length), true)
	})
}

// wholeString is the limit strcmp and strcasecmp pass: they read to the end of
// the shorter string and then let its length break the tie. It is not a large
// number, because a large number is still a limit, and a limit reached is what
// tells compareBytes to stop before the length rule.
const wholeString = -1

// clampLength reads strncmp's $length. PHP 8 raises a ValueError below zero;
// the bindings here clamp instead, and a length of zero compares nothing, which
// is the answer PHP gives for that call.
func clampLength(length int64) int {
	if length < 0 {
		return 0
	}
	// A length wider than any string this process holds bounds nothing: the
	// caller's own lengths bound the loop below.
	if length > int64(int(^uint(0)>>1)) {
		return int(^uint(0) >> 1)
	}
	return int(length)
}

// compareBytes is the one comparison the four functions differ only in the
// arguments to.
//
// The answer is the difference between the first two bytes that disagree, not a
// normalised -1/0/1, because that is what PHP's memcmp answers and a fixture
// recorded from php pins the number: strcmp("A", "a") is -32 there, not -1.
// Bytes compare unsigned, so "\xff" sorts after "\x01" rather than before it.
// Only a tie broken by length normalises, which is also PHP's reading:
// strcmp("a", "abcdefgh") is -1 and not -7.
func compareBytes(string1, string2 string, limit int, fold bool) int64 {
	n := len(string1)
	if len(string2) < n {
		n = len(string2)
	}

	bounded := limit != wholeString && limit < n
	if bounded {
		n = limit
	}

	for i := 0; i < n; i++ {
		a := string1[i]
		b := string2[i]
		if fold {
			a = lowerASCII(a)
			b = lowerASCII(b)
		}
		if a != b {
			return int64(a) - int64(b)
		}
	}

	// The compared run agreed. A strncmp that stopped at its own limit has
	// nothing left to say, whatever the strings do after it.
	if bounded {
		return 0
	}

	switch {
	case len(string1) < len(string2):
		return -1
	case len(string1) > len(string2):
		return 1
	}
	return 0
}

// lowerASCII folds one byte the way PHP's case-insensitive comparisons do:
// A-Z only. unicode.ToLower would fold a rune, and these functions never decode
// one - they walk bytes, which is why strcasecmp cannot see that "\xc3\x84" and
// "\xc3\xa4" are the same letter.
func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
