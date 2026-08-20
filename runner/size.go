package runner

import (
	"fmt"
	"strconv"
	"strings"
)

// Size is a byte count written the way a php.ini size is: a bare number of
// bytes, or a number with an M suffix for megabytes. PHP's K and G shorthands
// are rejected rather than guessed at, so a configuration file says what it
// means in one of two spellings and nothing else parses.
//
// The zero value means no limit, which is what 0 means in php.ini.
type Size int64

// megabyte is the multiplier of the M suffix, which is 1024*1024 in php.ini and
// not a million.
const megabyte = 1 << 20

// ParseSize reads a php.ini-style size. An empty value is no limit.
func ParseSize(s string) (Size, error) {
	text := strings.TrimSpace(s)
	if text == "" || text == "null" {
		return 0, nil
	}

	digits, isMegabytes := text, false
	if rest, ok := strings.CutSuffix(text, "M"); ok {
		digits, isMegabytes = rest, true
	} else if rest, ok := strings.CutSuffix(text, "m"); ok {
		digits, isMegabytes = rest, true
	}

	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("size %q: want a number of bytes or a number with an M suffix, such as 8M", s)
	}
	if value < 0 {
		return 0, fmt.Errorf("size %q: negative", s)
	}
	if isMegabytes {
		if value > (1<<63-1)/megabyte {
			return 0, fmt.Errorf("size %q: too large", s)
		}
		value *= megabyte
	}
	return Size(value), nil
}

// Bytes returns the limit as a byte count. Zero is no limit; a caller has to
// decide what that means for it.
func (s Size) Bytes() int64 { return int64(s) }

// Exceeds reports whether n is over the limit. A zero Size is no limit, so
// nothing exceeds it.
func (s Size) Exceeds(n int64) bool { return s > 0 && n > int64(s) }

// String writes the size back in the spelling it was read in: megabytes when it
// is a whole number of them, bytes otherwise.
func (s Size) String() string {
	if s != 0 && s%megabyte == 0 {
		return strconv.FormatInt(int64(s)/megabyte, 10) + "M"
	}
	return strconv.FormatInt(int64(s), 10)
}

// UnmarshalYAML reads a size from a configuration file, where it is written
// either as a bare number or as a quoted or unquoted "8M".
func (s *Size) UnmarshalYAML(data []byte) error {
	text := strings.Trim(strings.TrimSpace(string(data)), `"'`)
	size, err := ParseSize(text)
	if err != nil {
		return err
	}
	*s = size
	return nil
}

// MarshalYAML writes the size back as a string, so a round trip through a
// configuration file keeps the M suffix.
func (s Size) MarshalYAML() ([]byte, error) {
	return []byte(s.String()), nil
}
