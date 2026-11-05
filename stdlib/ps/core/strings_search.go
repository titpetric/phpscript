package core

import (
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
)

// phpStrpos implements strpos($haystack, $needle[, $offset]); the offset taken
// and the position returned count bytes, as PREG_OFFSET_CAPTURE
// reports and what substr consumes. mbStrpos counts characters.
func phpStrpos(haystack, needle string, offset ...int64) any {
	start, ok := searchStart(haystack, offset)
	if !ok {
		return false
	}
	i := strings.Index(haystack[start:], needle)
	if i < 0 {
		return false
	}
	return int64(i + start)
}

// phpStripos implements stripos($haystack, $needle[, $offset]). It folds both
// operands over A-Z only, which keeps their byte length and the offset still
// indexes the original haystack; PHP folds no wider than that either.
func phpStripos(haystack, needle string, offset ...int64) any {
	return phpStrpos(asciiLower(haystack), asciiLower(needle), offset...)
}

// phpStrrpos implements strrpos($haystack, $needle[, $offset]).
func phpStrrpos(haystack, needle string, offset ...int64) any {
	i := searchLast(haystack, needle, offset)
	if i < 0 {
		return false
	}
	return int64(i)
}

// phpStrripos implements strripos($haystack, $needle[, $offset]).
func phpStrripos(haystack, needle string, offset ...int64) any {
	i := searchLast(asciiLower(haystack), asciiLower(needle), offset)
	if i < 0 {
		return false
	}
	return int64(i)
}

// searchStart resolves the strpos-family $offset, counted in bytes, into the
// position the search begins at. A negative offset counts from the end; PHP 8
// raises a ValueError when it lands before the start, which phpscript clamps to
// 0 instead. The bool is false when the offset is past the end, where the
// search cannot match.
func searchStart(haystack string, offset []int64) (int, bool) {
	if len(offset) == 0 {
		return 0, true
	}
	start := offset[0]
	if start < 0 {
		start += int64(len(haystack))
		if start < 0 {
			start = 0
		}
	}
	if start > int64(len(haystack)) {
		return len(haystack), false
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
		start := int(o)
		i := strings.LastIndex(haystack[start:], needle)
		if i < 0 {
			return -1
		}
		return i + start
	}
	last := int64(len(haystack)) + offset[0]
	if last < 0 {
		last = 0
	}
	// A match inside the prefix that ends one needle past the cap starts at
	// or before it and LastIndex over that prefix answers directly.
	end := last + int64(len(needle))
	if end > int64(len(haystack)) {
		end = int64(len(haystack))
	}
	return strings.LastIndex(haystack[:end], needle)
}

// The three PHP 8 substring predicates are one-line wrappers so the published
// signature names $haystack and $needle, not the standard library's
// positional parameters.
func phpStrContains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

func phpStrStartsWith(haystack, needle string) bool { return strings.HasPrefix(haystack, needle) }

func phpStrEndsWith(haystack, needle string) bool { return strings.HasSuffix(haystack, needle) }

// phpSubstrCount implements substr_count($haystack, $needle[, $offset[, $length]]).
// strings.Count is non-overlapping, as PHP counts.
func phpSubstrCount(haystack, needle string, optional ...any) int64 {
	// PHP 8 raises a ValueError on an empty needle.
	if needle == "" {
		return 0
	}
	return int64(strings.Count(substrWindow(haystack, optional), needle))
}

// substrWindow cuts the haystack down to the optional $offset and $length, both
// counted in bytes. A negative offset counts from the end and a negative length
// stops that many bytes before it. PHP 8 raises a ValueError when either leaves
// the string; phpscript clamps.
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
