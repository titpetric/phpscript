// PHP's regular expressions are PCRE. Go's standard regexp is RE2, a different
// engine with a different bargain: RE2 guarantees a match runs in time linear
// in the length of the subject, and pays for that by leaving out every
// construct that would require backtracking. Two of those omissions show up in
// ordinary PHP code and preg_* compiles each pattern on whichever engine can
// express it.
//
//   - flags follow a /.../flags delimiter syntax, translated here to RE2's
//     inline (?flags) form,
//   - backreferences and lookaround are not expressible in RE2 at all.
//
// The second is not a corner case. minitpl's compiler pairs a template tag with
// its closing tag using `\1` and a pattern RE2 cannot compile appears in the
// first library this runtime targets, and reporting "no match" for it
// compiled every `{block}` and `{inline}` definition into garbage.
//
// RE2 stays the default: the faster of the two, and it cannot backtrack
// catastrophically. The fallback carries a match timeout for that reason.
//
// A third gap is in RE2's API and not in its syntax and it decides the
// engine per call and not per pattern. PHP's $offset moves where a match
// starts without moving where the subject begins and `^` and `\b` still see
// the real start of the string.
// Slicing subject[offset:] and adding the offset back gets both wrong, and RE2
// has no entry point that takes a start position. regexp2's
// FindStringMatchStartingAt does and a non-zero $offset runs on the
// backtracking engine even for a pattern RE2 compiled and a pattern
// can carry both engines.
//
// The two engines disagree about what an index counts: RE2 reports byte
// offsets, regexp2 matches over a []rune and reports rune indexes. PHP's
// offsets are byte offsets even under the /u modifier and every index out of
// regexp2 passes through runeOffsets before a script sees it.
//
// See docs/reference/extensions/regexp.md for the user-facing account.

package compat

import (
	"errors"
	"strings"
	"time"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/runner"
)

// registerRegex installs the preg_* shims onto rt. Each Runtime gets its own
// compiled-pattern cache and a pattern compiled for one request is not shared
// with another.
func registerRegex(rt *runner.Runtime) {
	cache := newRegexpCache()
	registerRegexConstants(rt)

	// preg_match_all fills $matches with every match of $pattern in $subject
	// and returns the count, in PREG_PATTERN_ORDER unless $flags
	// selects PREG_SET_ORDER, optionally starting at byte $offset; a pattern
	// that does not compile returns false and leaves $matches alone.
	rt.RegisterFunc("preg_match_all", cache.phpPregMatchAll)
	// preg_match fills $matches with the first match of $pattern in $subject
	// and returns 1, 0 when there is no match, optionally starting at byte
	// $offset; a pattern that does not compile, or an $offset outside
	// $subject, returns false.
	rt.RegisterFunc("preg_match", cache.phpPregMatch)
	// preg_replace replaces every match of $pattern in $subject with
	// $replacement, in which \1 and $1 both name a capture group.
	rt.RegisterFunc("preg_replace", cache.phpPregReplace)
	// preg_replace_callback replaces every match of $pattern in $subject with
	// what $callback returns for it, calling $callback once per match in
	// document order with the match array, at most $limit times, and
	// reporting the number of replacements through $count.
	rt.RegisterFunc("preg_replace_callback", func(pattern string, callback any, subject string, limit any, count func(any), flags any) (any, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return nil, errors.New("preg_replace_callback(): argument #2 ($callback) must be a valid callback")
		}
		return cache.phpPregReplaceCallback(pattern, fn, subject, limit, count, flags)
	})
	// preg_split splits $subject on every match of $pattern into at most
	// $limit pieces, the last of which holds the unsplit rest; $flags selects
	// PREG_SPLIT_NO_EMPTY, PREG_SPLIT_DELIM_CAPTURE and
	// PREG_SPLIT_OFFSET_CAPTURE.
	rt.RegisterFunc("preg_split", cache.phpPregSplit)
	// preg_quote escapes the characters that are special in a PCRE pattern,
	// plus $delimiter and a caller can splice a literal into one.
	rt.RegisterFunc("preg_quote", phpPregQuote)
}

// The PREG_* flag bits, with the values PHP defines. A script passes them as
// ordinary integers and combines them by adding and the values are part of the
// interface and no implementation detail: a $flags of 258 has to mean
// set order with offset capture.
const (
	pregPatternOrder    = 1
	pregSetOrder        = 2
	pregOffsetCapture   = 256
	pregUnmatchedAsNull = 512

	pregSplitNoEmpty       = 1
	pregSplitDelimCapture  = 2
	pregSplitOffsetCapture = 4
)

// registerRegexConstants defines the PREG_* constants. The two families share
// the low bits and are not interchangeable: PREG_SPLIT_NO_EMPTY and
// PREG_PATTERN_ORDER are both 1 and only the function that takes the flag
// decides what it means.
func registerRegexConstants(rt *runner.Runtime) {
	rt.SetConst("PREG_PATTERN_ORDER", int64(pregPatternOrder))
	rt.SetConst("PREG_SET_ORDER", int64(pregSetOrder))
	rt.SetConst("PREG_OFFSET_CAPTURE", int64(pregOffsetCapture))
	rt.SetConst("PREG_UNMATCHED_AS_NULL", int64(pregUnmatchedAsNull))

	rt.SetConst("PREG_SPLIT_NO_EMPTY", int64(pregSplitNoEmpty))
	rt.SetConst("PREG_SPLIT_DELIM_CAPTURE", int64(pregSplitDelimCapture))
	rt.SetConst("PREG_SPLIT_OFFSET_CAPTURE", int64(pregSplitOffsetCapture))
}

// backtrackTimeout bounds one match by the backtracking engine. A pattern that
// needs backreferences has no linear-time guarantee, and a request that hangs
// is worse than one that reports no match.
const backtrackTimeout = time.Second

// phpPregMatchAll implements preg_match_all($pattern, $subject, &$matches,
// $flags, $offset), returning the number of matches, or false for a pattern
// that does not compile. PHP leaves $matches untouched in that case rather
// than emptying it and the failure is distinguishable from "no matches".
//
// $flags selects PREG_PATTERN_ORDER (the default) or PREG_SET_ORDER, either
// combined with PREG_OFFSET_CAPTURE and PREG_UNMATCHED_AS_NULL.
func (c *regexpCache) phpPregMatchAll(pattern, subject string, matches func(any), flags any, offset any) any {
	re, err := c.compilePCRE(pattern)
	if err != nil {
		return false
	}
	bits := phpval.Int(flags)
	start, ok := matchOffset(subject, offset)
	if !ok {
		return false
	}
	all := re.findAllIndex(subject, start)
	groups := re.numGroups()
	if bits&pregSetOrder != 0 {
		writeRef(matches, setOrder(subject, all, groups, bits))
	} else {
		writeRef(matches, patternOrder(subject, all, groups, bits))
	}
	return int64(len(all))
}

// phpPregMatch implements preg_match($pattern, $subject, &$matches, $flags,
// $offset), returning 1 or 0 and filling $matches with the first match's
// groups. A pattern that does not compile, or an $offset outside $subject,
// returns false with $matches untouched, as PHP does.
func (c *regexpCache) phpPregMatch(pattern, subject string, matches func(any), flags any, offset any) any {
	re, err := c.compilePCRE(pattern)
	if err != nil {
		return false
	}
	bits := phpval.Int(flags)
	start, ok := matchOffset(subject, offset)
	if !ok {
		return false
	}
	idx := re.findIndex(subject, start)
	if idx == nil {
		writeRef(matches, []string(nil))
		return int64(0)
	}
	writeRef(matches, matchGroups(subject, idx, re.numGroups(), bits))
	return int64(1)
}

// phpPregReplace implements preg_replace($pattern, $replacement, $subject) for
// string arguments, converting PHP backreference syntax (\1 / $1) in the
// replacement to the ${1} form both engines expand.
func (c *regexpCache) phpPregReplace(pattern, replacement, subject string) string {
	re, err := c.compilePCRE(pattern)
	if err != nil {
		return subject
	}
	return re.replaceAll(subject, pcreReplacement(replacement))
}

// phpPregReplaceCallback implements preg_replace_callback($pattern, $callback,
// $subject, $limit, &$count, $flags). Neither engine's ReplaceAll can call
// back into the interpreter and this shim builds the replacement by hand from
// the match indexes. $limit is the number of replacements, -1 for every match;
// a pattern that does not compile returns null with $count zero, as PHP does.
func (c *regexpCache) phpPregReplaceCallback(pattern string, callback func(...any) (any, error), subject string, limit any, count func(any), flags any) (any, error) {
	replaced := int64(0)
	re, err := c.compilePCRE(pattern)
	if err != nil {
		writeRef(count, replaced)
		return nil, nil
	}
	bits := phpval.Int(flags)
	// An omitted $limit is PHP's -1 (replace everything). A passed 0 is not:
	// it names no replacements at all and nil and 0 have to differ here.
	max := int64(-1)
	if limit != nil {
		max = phpval.Int(limit)
	}
	groups := re.numGroups()

	var b strings.Builder
	last := 0
	for _, idx := range re.findAllIndex(subject, 0) {
		if max >= 0 && replaced >= max {
			break
		}
		b.WriteString(subject[last:idx[0]])
		value, err := callback(matchGroups(subject, idx, groups, bits))
		if err != nil {
			return nil, err
		}
		b.WriteString(phpval.String(value))
		last = idx[1]
		replaced++
	}
	b.WriteString(subject[last:])
	writeRef(count, replaced)
	return b.String(), nil
}

// phpPregSplit implements preg_split($pattern, $subject, $limit, $flags),
// returning the pieces between the matches of $pattern, or false for a pattern
// that does not compile.
//
// $limit caps the number of pieces, the last of which holds the unsplit rest;
// 0, -1 and an omitted argument all mean no limit, and any other negative
// value splits nothing, which is PHP's behaviour and not a reading of it.
// Only a piece that reaches the result counts against the limit, which is
// what makes PREG_SPLIT_NO_EMPTY and $limit compose the way PHP's do: an
// empty piece that NO_EMPTY drops leaves the limit alone.
func (c *regexpCache) phpPregSplit(pattern, subject string, limit any, flags any) any {
	re, err := c.compilePCRE(pattern)
	if err != nil {
		return false
	}
	pieces := splitPieces{flags: phpval.Int(flags)}
	remaining := phpval.Int(limit)
	if remaining == 0 {
		remaining = -1
	}

	last := 0
	for _, idx := range re.findAllIndex(subject, 0) {
		if remaining != -1 && remaining <= 1 {
			break
		}
		if pieces.add(subject, last, idx[0]) && remaining != -1 {
			remaining--
		}
		if pieces.flags&pregSplitDelimCapture != 0 {
			// Only the groups PHP would report: a trailing unmatched group is
			// not interleaved, while one in the middle is, as an empty string.
			n := participatingGroups(idx, len(idx)/2-1, 0)
			for g := 1; g < n; g++ {
				pieces.add(subject, idx[2*g], idx[2*g+1])
			}
		}
		last = idx[1]
	}
	pieces.add(subject, last, len(subject))
	return pieces.result()
}

// phpPregQuote escapes the characters that are special in a PCRE pattern and a
// caller can splice a literal into one.
func phpPregQuote(subject string, delimiter ...string) string {
	special := ".\\+*?[^]$(){}=!<>|:-#/"
	if len(delimiter) > 0 && delimiter[0] != "" {
		special += delimiter[0][:1]
	}
	var b strings.Builder
	b.Grow(len(subject))
	for i := 0; i < len(subject); i++ {
		if strings.IndexByte(special, subject[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(subject[i])
	}
	return b.String()
}
