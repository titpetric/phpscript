package core

import (
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// STR_PAD_* selectors, as PHP numbers them.
const (
	strPadLeft  = 0
	strPadRight = 1
	strPadBoth  = 2
)

// phpStrrev reverses bytes, as PHP's strrev does and a multi-byte character
// comes out as its bytes in reverse, which is not valid UTF-8.
func phpStrrev(str string) string {
	b := []byte(str)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

// phpStrSplit returns the chunks as a []string for the reason phpExplode does:
// the loop allocates exactly that slice and the VM indexes it like any array.
func phpStrSplit(str string, length ...int64) []string {
	// PHP 8.2 returns an empty array for an empty subject; older versions
	// returned a single empty chunk.
	if str == "" {
		return []string{}
	}
	n := int64(1)
	// PHP 8 raises a ValueError below 1; phpscript clamps.
	if len(length) > 0 && length[0] > 1 {
		n = length[0]
	}
	return byteSplit(str, n)
}

// phpStrPad implements str_pad($string, $length, $pad_string, $pad_type). Go
// allows one variadic and both optional arguments arrive together: the pad
// string first, the STR_PAD_* selector second.
func phpStrPad(str string, length int64, optional ...any) string {
	pad := " "
	if len(optional) > 0 {
		pad = phpval.String(optional[0])
	}
	padType := int64(strPadRight)
	if len(optional) > 1 {
		padType = phpval.Int(optional[1])
	}
	diff := length - int64(len(str))
	if diff <= 0 || pad == "" {
		return str
	}
	switch padType {
	case strPadLeft:
		return padTo(pad, int(diff)) + str
	case strPadBoth:
		// An odd difference puts the extra byte on the right, as PHP halves
		// downwards.
		left := diff / 2
		return padTo(pad, int(left)) + str + padTo(pad, int(diff-left))
	default:
		return str + padTo(pad, int(diff))
	}
}

// padTo repeats pad to exactly n bytes, cutting the last repetition short.
func padTo(pad string, n int) string {
	if n <= 0 {
		return ""
	}
	whole := strings.Repeat(pad, n/len(pad)+1)
	return whole[:n]
}

// phpSubstrReplace implements substr_replace($string, $replace, $offset[, $length]).
// PHP clamps an out-of-range offset here and raises nothing and this does too.
func phpSubstrReplace(str, replace string, offset int64, length ...int64) string {
	n := int64(len(str))
	start := offset
	if start < 0 {
		start += n
		if start < 0 {
			start = 0
		}
	}
	if start > n {
		start = n
	}
	end := n
	if len(length) > 0 {
		// A negative length is a distance from the end of the string, with no
		// a count of characters to drop.
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
		end = start
	}
	return str[:start] + replace + str[end:]
}

// phpStrReplace implements str_replace where search may be a string or a
// collection (with a scalar or collection replacement), and subject is a string.
func phpStrReplace(search, replace, subject any) (string, error) {
	out, err := phpval.StringContext(subject)
	if err != nil {
		return "", err
	}
	if model.IsCollection(search) {
		replIsList := model.IsCollection(replace)
		repl, err := phpval.Strings(replace)
		if err != nil {
			return "", err
		}
		// A scalar replacement converts once, not once per search term.
		scalar := ""
		if !replIsList {
			if scalar, err = phpval.StringContext(replace); err != nil {
				return "", err
			}
		}
		terms, err := phpval.Strings(search)
		if err != nil {
			return "", err
		}
		for i, s := range terms {
			r := scalar
			if replIsList {
				r = ""
				if i < len(repl) {
					r = repl[i]
				}
			}
			out = strings.ReplaceAll(out, s, r)
		}
		return out, nil
	}
	from, err := phpval.StringContext(search)
	if err != nil {
		return "", err
	}
	to, err := phpval.StringContext(replace)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(out, from, to), nil
}

func phpImplode(separator, array any) (string, error) {
	// implode($separator, $array) or implode($array), where the single
	// array argument arrives in $separator.
	if model.IsCollection(separator) {
		parts, err := phpval.Strings(separator)
		if err != nil {
			return "", err
		}
		return strings.Join(parts, ""), nil
	}
	glue, err := phpval.StringContext(separator)
	if err != nil {
		return "", err
	}
	// A []string joins without the per-element conversion phpval.Strings would do.
	if parts, ok := array.([]string); ok {
		return strings.Join(parts, glue), nil
	}
	parts, err := phpval.Strings(array)
	if err != nil {
		return "", err
	}
	return strings.Join(parts, glue), nil
}

// phpExplode returns the parts as a []string: strings.Split already allocated
// exactly that and handing it straight to the VM costs nothing beyond the split
// itself. The VM indexes, iterates and destructures it like any array.
func phpExplode(separator, str string, limit ...int64) []string {
	parts := strings.Split(str, separator)
	if len(limit) > 0 && limit[0] > 0 && int64(len(parts)) > limit[0] {
		tail := strings.Join(parts[limit[0]-1:], separator)
		parts = parts[:limit[0]]
		parts[limit[0]-1] = tail
	}
	return parts
}
