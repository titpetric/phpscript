package shared

// URLDecode decodes an application/x-www-form-urlencoded string.
//
// Lenient, because PHP is: `a=%zz` decodes to the literal `%zz`. net/url
// refuses the same input and drops the whole pair, which loses a field a
// browser did send.
func URLDecode(s string) string {
	return decode(s, true)
}

// RawURLDecode decodes an RFC 3986 string: as URLDecode, but `+` is a plus.
func RawURLDecode(s string) string {
	return decode(s, false)
}

func decode(s string, plusIsSpace bool) string {
	// The common case for a key, so scan before allocating.
	if !needsDecoding(s, plusIsSpace) {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '+' && plusIsSpace:
			out = append(out, ' ')
		case s[i] == '%' && i+2 < len(s):
			hi, hiOK := unhex(s[i+1])
			lo, loOK := unhex(s[i+2])
			if !hiOK || !loOK {
				out = append(out, s[i])
				continue
			}
			out = append(out, hi<<4|lo)
			i += 2
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}

func needsDecoding(s string, plusIsSpace bool) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '%' || (s[i] == '+' && plusIsSpace) {
			return true
		}
	}
	return false
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
