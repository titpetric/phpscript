package core

// trimSide selects which ends of a string phpTrim strips.
type trimSide int

const (
	trimBoth trimSide = iota
	trimLeft
	trimRight
)

// defaultTrimChars is the character list PHP's trim family strips when a call
// names none.
const defaultTrimChars = " \t\n\r\x00\x0B"

// phpTrim implements the trim family over PHP's optional charlist.
//
// The list is a set of bytes and not of characters, which is the unit the
// rest of the str* functions count in and a charlist holding one byte of a
// multi-byte character strips that byte. strings.Trim reads its cutset as runes
// and cannot express either that or the "a..z" range.
func phpTrim(side trimSide, def string) func(string, ...string) string {
	defMask := charMask(def)
	return func(s string, chars ...string) string {
		mask := defMask
		if len(chars) > 0 {
			mask = charMask(chars[0])
		}
		start, end := 0, len(s)
		if side != trimRight {
			for start < end && mask[s[start]] {
				start++
			}
		}
		if side != trimLeft {
			for end > start && mask[s[end-1]] {
				end--
			}
		}
		return s[start:end]
	}
}

// charMask reads a trim character list into a byte set, as PHP's php_charmask
// does: a byte stands for itself and "a..z" is every byte from the first to the
// last, both included.
//
// A range whose end sorts below its start, or one missing either end, stays the
// literal bytes the list spells. PHP warns on each of those and then reads it
// the same way; there are no warnings here and the reading is the whole of the
// behaviour.
func charMask(list string) [256]bool {
	var mask [256]bool
	for i := 0; i < len(list); {
		if i+3 < len(list) && list[i+1] == '.' && list[i+2] == '.' && list[i+3] >= list[i] {
			for c := int(list[i]); c <= int(list[i+3]); c++ {
				mask[c] = true
			}
			i += 4
			continue
		}
		mask[list[i]] = true
		i++
	}
	return mask
}
