package core

// trimSide selects which ends of a string phpTrim strips.
type trimSide int

const (
	trimBoth trimSide = iota
	trimLeft
	trimRight
)

// defaultTrimChars is the character list trim strips when a call names none.
const defaultTrimChars = " \t\n\r\x00\x0B"

// phpTrim builds a trim over PHP's optional byte charlist, which strings.Trim cannot express.
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

// charMask reads a trim list into a byte set, expanding each "a..z" range.
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
