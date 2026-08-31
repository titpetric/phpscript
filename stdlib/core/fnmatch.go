package core

import (
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
)

// FNM_* flags, as PHP numbers them. They are the matcher's own parameters
// rather than extra surface: every one of them changes a decision the loop in
// fnmatchBytes already has to make.
const (
	fnmPathname = 1
	fnmNoescape = 2
	fnmPeriod   = 4
	fnmCasefold = 16
)

// phpFnmatch reports whether $string matches the shell wildcard $pattern.
//
// Go's path.Match is not the primitive underneath. It disagrees with fnmatch on
// three points: its * and ? never cross a separator, which is fnmatch's
// FNM_PATHNAME behaviour rather than its default; it spells a negated class
// [^...] where fnmatch also accepts [!...]; and it reports a malformed pattern
// as an error where fnmatch simply does not match.
func phpFnmatch(pattern, str string, flags ...any) bool {
	var mode int64
	if len(flags) > 0 {
		mode = phpval.Int(flags[0])
	}
	// Folding both sides once is cheaper than folding per byte comparison,
	// and it folds the pattern's bracket expressions along with everything
	// else, which a per-byte comparison would have to do by hand.
	if mode&fnmCasefold != 0 {
		pattern = strings.ToLower(pattern)
		str = strings.ToLower(str)
	}
	return fnmatchBytes(pattern, str, mode)
}

// fnmatchBytes matches pattern against str.
//
// The walk is iterative with one saved backtrack point per star, so a run of
// consecutive stars costs the same as one and the match is linear in the common
// case. Nothing recurses.
//
// start tracks whether the current position in str is one where a leading
// period is special, which is what FNM_PERIOD asks about: the beginning of the
// string, and under FNM_PATHNAME the byte after every separator.
func fnmatchBytes(pattern, str string, mode int64) bool {
	pathname := mode&fnmPathname != 0
	period := mode&fnmPeriod != 0
	noescape := mode&fnmNoescape != 0

	var p, s int
	start := true

	// star is the pattern offset just past the last '*' seen, negative when
	// no star is open, in which case a mismatch fails outright. next is the
	// str offset that star resumes from, starPos the offset it started at,
	// and starStart the leading-period state there.
	star, next, starPos, starStart := -1, 0, 0, false

	for s < len(str) {
		if p < len(pattern) {
			matched := true
			switch c := pattern[p]; c {
			case '*':
				// The star matches nothing to begin with; the
				// bytes it swallows are taken one at a time on
				// the way back, below, which is where its two
				// limits are enforced.
				star, next, starPos, starStart = p+1, s, s, start
				p++
				continue
			case '?':
				switch {
				case period && start && str[s] == '.':
					matched = false
				case pathname && str[s] == '/':
					matched = false
				}
			case '[':
				end, in, closed := fnmatchClass(pattern, p, str[s], noescape)
				switch {
				case !closed:
					// An unterminated '[' is not an error;
					// fnmatch reads it as a literal.
					matched = str[s] == '['
				case period && start && str[s] == '.':
					matched = false
				case pathname && str[s] == '/':
					matched = false
				case in:
					start = str[s] == '/' && pathname
					p = end
					s++
					continue
				default:
					matched = false
				}
			case '\\':
				if !noescape && p+1 < len(pattern) {
					if pattern[p+1] == str[s] {
						start = pathname && str[s] == '/'
						p += 2
						s++
						continue
					}
					matched = false
					break
				}
				matched = c == str[s]
			default:
				matched = c == str[s]
			}
			if matched {
				start = pathname && str[s] == '/'
				p++
				s++
				continue
			}
		}

		// Either the pattern ran out with string left over, or the byte
		// did not match. Resume the open star one byte further along.
		if star < 0 {
			return false
		}
		if pathname && str[next] == '/' {
			// A star does not cross a separator under
			// FNM_PATHNAME, so there is no further resume point.
			return false
		}
		if period && starStart && next == starPos && str[next] == '.' {
			// Nor does it swallow a leading period under
			// FNM_PERIOD. Matching nothing was already tried.
			return false
		}
		next++
		// The star has consumed at least one byte, so the resume point
		// is never a leading-period position.
		p, s, start = star, next, false
	}

	// The string is spent; the pattern matches only if what is left of it
	// can match the empty string, which only trailing stars do.
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

// fnmatchClass matches one bracket expression starting at pattern[open], which
// is the '['. It reports the offset just past the closing ']', whether b is in
// the set, and whether the expression was closed at all.
//
// PHP accepts both [!...] and [^...] for a negated class, so both are read
// here. A ']' in the first position is a literal, as POSIX has it.
func fnmatchClass(pattern string, open int, b byte, noescape bool) (end int, in, closed bool) {
	i := open + 1
	negate := false
	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		negate = true
		i++
	}

	found := false
	for first := true; i < len(pattern); first = false {
		if pattern[i] == ']' && !first {
			return i + 1, found != negate, true
		}

		lo := pattern[i]
		if lo == '\\' && !noescape && i+1 < len(pattern) {
			i++
			lo = pattern[i]
		}
		i++

		// A '-' is a range only when something other than the closing
		// ']' follows it; otherwise it is a member of the set.
		if i+1 < len(pattern) && pattern[i] == '-' && pattern[i+1] != ']' {
			i++
			hi := pattern[i]
			if hi == '\\' && !noescape && i+1 < len(pattern) {
				i++
				hi = pattern[i]
			}
			i++
			if lo <= b && b <= hi {
				found = true
			}
			continue
		}
		if lo == b {
			found = true
		}
	}

	return open, false, false
}
