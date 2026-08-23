// PHP's regular expressions are PCRE. Go's standard regexp is RE2, a different
// engine with a different bargain: RE2 guarantees a match runs in time linear
// in the length of the subject, and pays for that by leaving out every
// construct that would require backtracking. Two of those omissions show up in
// ordinary PHP code, so preg_* is compiled by whichever engine can express the
// pattern.
//
//   - flags follow a /.../flags delimiter syntax, translated here to RE2's
//     inline (?flags) form,
//   - backreferences and lookaround are not expressible in RE2 at all.
//
// The second is not a corner case. minitpl's compiler pairs a template tag with
// its closing tag using `\1`, so a pattern RE2 cannot compile appears in the
// first library phpscript was built to run, and reporting "no match" for it
// compiled every `{block}` and `{inline}` definition into garbage.
//
// RE2 stays the default: it is the faster of the two and cannot backtrack
// catastrophically, which is also why the fallback carries a match timeout.
//
// A third omission decides the engine at match time rather than at compile
// time. PHP's $offset moves where a match starts without moving where the
// subject begins, so `^` and `\b` still see the real start of the string.
// Slicing subject[offset:] and adding the offset back gets both wrong, and RE2
// has no entry point that takes a start position. regexp2's
// FindStringMatchStartingAt does, so a non-zero $offset is routed to the
// backtracking engine even for a pattern RE2 compiled, which is why a pattern
// can carry both engines.
//
// The two engines disagree about what an index counts: RE2 reports byte
// offsets, regexp2 matches over a []rune and reports rune indexes. PHP's
// offsets are byte offsets even under the /u modifier, so every index out of
// regexp2 goes through runeOffsets before a script sees it.
//
// See docs/reference/extensions/regexp.md for the user-facing account.

package compat

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dlclark/regexp2"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/runner"
)

// registerRegex installs the preg_* shims onto rt. Each Runtime gets its own
// compiled-pattern cache, so a pattern compiled for one request is not shared
// with another.
func registerRegex(rt *runner.Runtime) {
	cache := newRegexpCache()
	registerRegexConstants(rt)

	// preg_match_all fills $matches with every match of $pattern in $subject and returns how many there were, in PREG_PATTERN_ORDER unless $flags selects PREG_SET_ORDER, optionally starting at byte $offset; a pattern that does not compile returns false and leaves $matches alone.
	rt.RegisterFunc("preg_match_all", cache.phpPregMatchAll)
	// preg_match fills $matches with the first match of $pattern in $subject and returns 1, 0 when there is no match, optionally starting at byte $offset; a pattern that does not compile, or an $offset outside $subject, returns false.
	rt.RegisterFunc("preg_match", cache.phpPregMatch)
	// preg_replace replaces every match of $pattern in $subject with $replacement, in which \1 and $1 both name a capture group.
	rt.RegisterFunc("preg_replace", cache.phpPregReplace)
	// preg_replace_callback replaces every match of $pattern in $subject with what $callback returns for it, calling $callback once per match in document order with the match array, at most $limit times, and reporting the number of replacements through $count.
	rt.RegisterFunc("preg_replace_callback", func(pattern string, callback any, subject string, limit any, count func(any), flags any) (any, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return nil, errors.New("preg_replace_callback(): argument #2 ($callback) must be a valid callback")
		}
		return cache.phpPregReplaceCallback(pattern, fn, subject, limit, count, flags)
	})
	// preg_split splits $subject on every match of $pattern into at most $limit pieces, the last of which holds the remainder; $flags selects PREG_SPLIT_NO_EMPTY, PREG_SPLIT_DELIM_CAPTURE and PREG_SPLIT_OFFSET_CAPTURE.
	rt.RegisterFunc("preg_split", cache.phpPregSplit)
	// preg_quote escapes the characters that are special in a PCRE pattern, plus $delimiter, so a literal can be spliced into one.
	rt.RegisterFunc("preg_quote", phpPregQuote)
}

// The PREG_* flag bits, with the values PHP defines. A script passes them as
// ordinary integers, so the values are the interface and not an implementation
// detail: PREG_OFFSET_CAPTURE|PREG_SET_ORDER has to arrive here as 258.
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
// PREG_PATTERN_ORDER are both 1, so only the function a flag is passed to
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
// than emptying it, so the failure is distinguishable from "no matches".
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

// patternOrder fills $matches with one entry per capture group: $matches[0]
// holds every whole match and $matches[g] every capture of group g. The
// columns are present even when nothing matched, so a caller may index
// $matches[1] before it checks the count.
//
// Without a flag that changes the entries, the columns are []string. Both
// levels are indexed and iterated by the VM exactly like nested PHP arrays,
// and the columns are plain string slices, so a match set of g groups over n
// matches costs g+1 allocations instead of the 2(g+1) plus 2n interface boxes
// an *model.Array pair would. PREG_OFFSET_CAPTURE boxes every entry anyway and
// takes the []any branch.
func patternOrder(subject string, all [][]int, groups int, flags int64) []any {
	out := make([]any, 0, groups+1)
	boxed := flags&(pregOffsetCapture|pregUnmatchedAsNull) != 0
	for g := 0; g <= groups; g++ {
		if boxed {
			col := make([]any, len(all))
			for i, idx := range all {
				col[i] = matchValue(subject, idx, g, flags)
			}
			out = append(out, col)
			continue
		}
		col := make([]string, len(all))
		for i, idx := range all {
			col[i] = groupText(subject, idx, g)
		}
		out = append(out, col)
	}
	return out
}

// setOrder fills $matches with one entry per match, each holding the whole
// match followed by its groups: the transpose of patternOrder.
func setOrder(subject string, all [][]int, groups int, flags int64) []any {
	out := make([]any, len(all))
	for i, idx := range all {
		out[i] = matchGroups(subject, idx, groups, flags)
	}
	return out
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

// matchOffset resolves preg_match's $offset argument to a byte offset into
// subject. A negative offset counts from the end. An offset outside the
// subject is not a failed match but a failed call: PHP returns false, so the
// second result reports whether the offset was usable at all.
func matchOffset(subject string, offset any) (int, bool) {
	if offset == nil {
		return 0, true
	}
	start := int(phpval.Int(offset))
	if start < 0 {
		start += len(subject)
	}
	if start < 0 || start > len(subject) {
		return 0, false
	}
	return start, true
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
// back into the interpreter, so the replacement is assembled by hand from the
// match indexes. $limit is the number of replacements, -1 for all of them; a
// pattern that does not compile returns null with $count zero, as PHP does.
func (c *regexpCache) phpPregReplaceCallback(pattern string, callback func(...any) (any, error), subject string, limit any, count func(any), flags any) (any, error) {
	replaced := int64(0)
	re, err := c.compilePCRE(pattern)
	if err != nil {
		writeRef(count, replaced)
		return nil, nil
	}
	bits := phpval.Int(flags)
	// An omitted $limit is PHP's -1 (replace everything). A passed 0 is not:
	// it asks for no replacements at all, so nil and 0 have to differ here.
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
		value, err := callback(matchGroups(subject, idx, groups, bits))
		if err != nil {
			return nil, err
		}
		b.WriteString(subject[last:idx[0]])
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
// $limit caps the number of pieces, the last of which holds the unsplit
// remainder; 0, -1 and an omitted argument all mean no limit. Only a piece
// that reaches the result counts against it, which is what makes
// PREG_SPLIT_NO_EMPTY and $limit compose the way PHP's do: an empty piece that
// NO_EMPTY drops leaves the limit where it was.
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
			// Only the groups PHP would report: a trailing group that did not
			// participate is not interleaved, while one in the middle is, as
			// an empty string.
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

// splitPieces accumulates preg_split's result. It holds a []string while the
// flags allow it, for the same reason patternOrder does, and switches to []any
// only for PREG_SPLIT_OFFSET_CAPTURE, where every piece is a pair anyway.
type splitPieces struct {
	flags int64
	text  []string
	pairs []any
}

// add appends the piece of subject between the byte offsets start and end, and
// reports whether it landed in the result: PREG_SPLIT_NO_EMPTY drops an empty
// one. A start of -1 is a capture group that did not participate, which is an
// empty piece at offset -1.
func (s *splitPieces) add(subject string, start, end int) bool {
	piece := ""
	if start >= 0 {
		piece = subject[start:end]
	}
	if s.flags&pregSplitNoEmpty != 0 && piece == "" {
		return false
	}
	if s.flags&pregSplitOffsetCapture != 0 {
		s.pairs = append(s.pairs, []any{piece, int64(start)})
		return true
	}
	s.text = append(s.text, piece)
	return true
}

// result returns the pieces in the shape the script sees. An empty result is
// an empty array rather than null, which is what PHP returns when every piece
// was dropped.
func (s *splitPieces) result() any {
	if s.flags&pregSplitOffsetCapture != 0 {
		if s.pairs == nil {
			return []any{}
		}
		return s.pairs
	}
	if s.text == nil {
		return []string{}
	}
	return s.text
}

// matchGroups renders one match as the row a script reads: index 0 is the
// whole match and index g is group g.
//
// PHP drops the trailing groups that did not participate, so count($m) is
// smaller for a match that ended before the last optional group;
// PREG_UNMATCHED_AS_NULL turns those into nulls instead and then they all
// stay. The flagless case is a []string for the reason patternOrder gives.
func matchGroups(subject string, idx []int, groups int, flags int64) any {
	n := participatingGroups(idx, groups, flags)
	if flags&(pregOffsetCapture|pregUnmatchedAsNull) == 0 {
		row := make([]string, n)
		for g := 0; g < n; g++ {
			row[g] = groupText(subject, idx, g)
		}
		return row
	}
	row := make([]any, n)
	for g := 0; g < n; g++ {
		row[g] = matchValue(subject, idx, g, flags)
	}
	return row
}

// participatingGroups reports how many entries of a match row PHP keeps: every
// group up to the last one that participated, and always at least the whole
// match. PREG_UNMATCHED_AS_NULL keeps them all, since a null records the group
// that did not participate rather than hiding it.
func participatingGroups(idx []int, groups int, flags int64) int {
	if flags&pregUnmatchedAsNull != 0 {
		return groups + 1
	}
	n := groups + 1
	for n > 1 && groupStart(idx, n-1) < 0 {
		n--
	}
	return n
}

// groupStart returns the byte offset group g matched at, or -1 when the group
// did not participate or the pattern has no such group.
func groupStart(idx []int, g int) int {
	if 2*g+1 >= len(idx) {
		return -1
	}
	return idx[2*g]
}

// groupText returns the text group g matched, empty for a group that did not
// participate.
func groupText(subject string, idx []int, g int) string {
	start := groupStart(idx, g)
	if start < 0 {
		return ""
	}
	return subject[start:idx[2*g+1]]
}

// matchValue renders group g the way $matches holds it under flags:
// PREG_OFFSET_CAPTURE makes it a pair of the text and its byte offset, -1 for
// a group that did not participate, and PREG_UNMATCHED_AS_NULL makes that
// group's text null instead of empty.
func matchValue(subject string, idx []int, g int, flags int64) any {
	start := groupStart(idx, g)
	var text any = ""
	switch {
	case start >= 0:
		text = subject[start:idx[2*g+1]]
	case flags&pregUnmatchedAsNull != 0:
		text = nil
	}
	if flags&pregOffsetCapture != 0 {
		return []any{text, int64(start)}
	}
	return text
}

// phpPregQuote escapes the characters that are special in a PCRE pattern, so a
// literal can be spliced into one.
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

// writeRef assigns v to a by-reference out parameter. The setter is nil when
// the script left the argument off, which is legal for every one of them.
func writeRef(set func(any), v any) {
	if set != nil {
		set(v)
	}
}

// pattern is one compiled PCRE, held by whichever engine can express it. The
// shims above work only through this type, so which engine ran a match is not
// visible in their results.
//
// A pattern RE2 compiled still keeps its body and flags, because a non-zero
// $offset has to run on the backtracking engine whatever compiled it (see the
// file header). That second compile happens once, on the first call that needs
// it, so patterns nobody passes an offset to never pay for it.
type pattern struct {
	re2   *regexp.Regexp
	body  string
	flags string

	once         sync.Once
	backtrack    *regexp2.Regexp
	backtrackErr error
}

// backtracker returns the backtracking engine for this pattern, compiling it
// on first use.
func (p *pattern) backtracker() (*regexp2.Regexp, error) {
	p.once.Do(func() {
		re, err := regexp2.Compile(p.body, backtrackOptions(p.flags))
		if err != nil {
			p.backtrackErr = err
			return
		}
		re.MatchTimeout = backtrackTimeout
		p.backtrack = re
	})
	return p.backtrack, p.backtrackErr
}

// numGroups reports the number of capturing groups.
func (p *pattern) numGroups() int {
	if p.re2 != nil {
		return p.re2.NumSubexp()
	}
	re, err := p.backtracker()
	if err != nil {
		return 0
	}
	return backtrackGroupCount(re)
}

// backtrackGroupCount counts the capturing groups of a regexp2 pattern.
// regexp2 numbers group 0 (the whole match) among its group numbers, and the
// highest number is the count of capturing groups.
func backtrackGroupCount(re *regexp2.Regexp) int {
	highest := 0
	for _, n := range re.GetGroupNumbers() {
		if n > highest {
			highest = n
		}
	}
	return highest
}

// findIndex returns the first match at or after byte offset start as byte
// index pairs, two per group: idx[2g] and idx[2g+1] bound group g, and both
// are -1 for a group that did not participate. Nil means no match.
func (p *pattern) findIndex(subject string, start int) []int {
	if p.re2 != nil && start == 0 {
		return p.re2.FindStringSubmatchIndex(subject)
	}
	re, err := p.backtracker()
	if err != nil {
		return nil
	}
	match, err := re.FindStringMatchStartingAt(subject, start)
	if err != nil || match == nil {
		return nil
	}
	return backtrackIndex(match, backtrackGroupCount(re), newRuneOffsets(subject))
}

// findAllIndex returns every match at or after byte offset start, in the order
// they occur, in the shape findIndex returns one.
func (p *pattern) findAllIndex(subject string, start int) [][]int {
	if p.re2 != nil && start == 0 {
		return p.re2.FindAllStringSubmatchIndex(subject, -1)
	}
	re, err := p.backtracker()
	if err != nil {
		return nil
	}
	groups := backtrackGroupCount(re)
	offsets := newRuneOffsets(subject)
	var out [][]int
	match, err := re.FindStringMatchStartingAt(subject, start)
	for err == nil && match != nil {
		out = append(out, backtrackIndex(match, groups, offsets))
		match, err = re.FindNextMatch(match)
	}
	return out
}

// replaceAll replaces every match with repl, whose ${n} references expand to
// the corresponding group.
func (p *pattern) replaceAll(subject, repl string) string {
	if p.re2 != nil {
		return p.re2.ReplaceAllString(subject, repl)
	}
	re, err := p.backtracker()
	if err != nil {
		return subject
	}
	out, err := re.Replace(subject, repl, -1, -1)
	if err != nil {
		return subject
	}
	return out
}

// backtrackIndex flattens a regexp2 match into the byte index pairs RE2
// returns. regexp2 counts in runes, so every index goes through offsets; a
// group with no captures did not participate and is reported as -1, -1.
func backtrackIndex(match *regexp2.Match, groups int, offsets runeOffsets) []int {
	idx := make([]int, 2*(groups+1))
	for g := 0; g <= groups; g++ {
		idx[2*g], idx[2*g+1] = -1, -1
		group := match.GroupByNumber(g)
		if group == nil || len(group.Captures) == 0 {
			continue
		}
		idx[2*g] = offsets.byteOffset(group.Index)
		idx[2*g+1] = offsets.byteOffset(group.Index + group.Length)
	}
	return idx
}

// runeOffsets converts the rune indexes regexp2 reports into the byte offsets
// PHP reports. PHP's offsets are byte offsets even under the /u modifier, so
// the two agree only while the subject is ASCII, and a match after a two-byte
// character is off by one per preceding multibyte character otherwise.
type runeOffsets struct {
	size int // len(subject), the offset a rune index past the end converts to
	// byteAt[i] is the byte offset of rune i, with a final entry of size so
	// that a match ending on the last rune converts without a special case. It
	// stays nil for an ASCII subject, where the two indexes are equal and the
	// table would be pure overhead.
	byteAt []int
}

// newRuneOffsets builds the conversion for subject.
func newRuneOffsets(subject string) runeOffsets {
	offsets := runeOffsets{size: len(subject)}
	ascii := true
	for i := 0; i < len(subject); i++ {
		if subject[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return offsets
	}
	byteAt := make([]int, 0, utf8.RuneCountInString(subject)+1)
	for i := range subject {
		byteAt = append(byteAt, i)
	}
	offsets.byteAt = append(byteAt, len(subject))
	return offsets
}

// byteOffset converts a rune index into a byte offset. A negative index is a
// group that did not participate, which stays negative; an index past the last
// rune is the end of the subject.
func (r runeOffsets) byteOffset(runeIndex int) int {
	switch {
	case runeIndex < 0:
		return -1
	case r.byteAt == nil:
		if runeIndex > r.size {
			return r.size
		}
		return runeIndex
	case runeIndex >= len(r.byteAt):
		return r.size
	}
	return r.byteAt[runeIndex]
}

type regexpCache struct {
	mu    sync.Mutex
	cache map[string]regexpCacheEntry
}

type regexpCacheEntry struct {
	pattern *pattern
	err     error
}

func newRegexpCache() *regexpCache {
	return &regexpCache{cache: map[string]regexpCacheEntry{}}
}

func (c *regexpCache) compilePCRE(source string) (*pattern, error) {
	c.mu.Lock()
	entry, ok := c.cache[source]
	c.mu.Unlock()
	if ok {
		return entry.pattern, entry.err
	}

	compiled, err := compilePCRE(source)

	c.mu.Lock()
	c.cache[source] = regexpCacheEntry{pattern: compiled, err: err}
	c.mu.Unlock()

	return compiled, err
}

// compilePCRE translates a PCRE pattern (with /.../flags delimiters) into a
// compiled pattern, preferring RE2 and falling back to the backtracking engine
// for the constructs RE2 has no way to express.
func compilePCRE(source string) (*pattern, error) {
	body, flags := splitPCRE(source)
	body = sanitizeEscapes(body)
	p := &pattern{body: body, flags: flags}
	if !needsBacktracking(body) {
		re2body := body
		if goFlags := translateFlags(flags); goFlags != "" {
			re2body = "(?" + goFlags + ")" + body
		}
		if re, err := regexp.Compile(re2body); err == nil {
			p.re2 = re
			return p, nil
		}
	}
	// Nothing else can run this pattern, so compile the backtracking engine
	// now: a syntax error has to reach the caller as a compile failure, not as
	// the no-match a first-use compile would look like.
	if _, err := p.backtracker(); err != nil {
		return nil, err
	}
	return p, nil
}

// splitPCRE separates a /body/flags pattern into body and flag characters. If
// the pattern is not delimited it is treated as a bare body with no flags.
func splitPCRE(p string) (body, flags string) {
	if len(p) < 2 {
		return p, ""
	}
	delim := p[0]
	close := closingDelim(delim)
	end := strings.LastIndexByte(p, close)
	if end <= 0 {
		return p, ""
	}
	return p[1:end], p[end+1:]
}

func closingDelim(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '{':
		return '}'
	case '[':
		return ']'
	case '<':
		return '>'
	default:
		return open
	}
}

// translateFlags maps PCRE modifiers to RE2 inline flags. s, U, m, i, x map
// directly; others are ignored.
func translateFlags(flags string) string {
	var b strings.Builder
	for _, f := range flags {
		switch f {
		case 's', 'U', 'm', 'i', 'x':
			b.WriteRune(f)
		}
	}
	return b.String()
}

// backtrackOptions maps PCRE modifiers to regexp2 options. PCRE's `s` makes the
// dot match a newline, which regexp2 spells Singleline; `U` (ungreedy) has no
// equivalent and is dropped, so a pattern needing both it and a backreference
// matches greedily.
func backtrackOptions(flags string) regexp2.RegexOptions {
	options := regexp2.RegexOptions(0)
	for _, f := range flags {
		switch f {
		case 's':
			options |= regexp2.Singleline
		case 'm':
			options |= regexp2.Multiline
		case 'i':
			options |= regexp2.IgnoreCase
		case 'x':
			options |= regexp2.IgnorePatternWhitespace
		}
	}
	return options
}

// needsBacktracking reports whether the pattern uses a construct RE2 cannot
// express: a \1..\9 backreference, or a lookahead/lookbehind group. Both are
// deliberate RE2 omissions rather than gaps, so detecting them up front avoids
// a compile attempt that is certain to fail.
func needsBacktracking(body string) bool {
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '\\':
			if i+1 < len(body) {
				if n := body[i+1]; n >= '1' && n <= '9' {
					return true
				}
				i++ // the escaped character is not itself syntax
			}
		case '(':
			if strings.HasPrefix(body[i:], "(?=") || strings.HasPrefix(body[i:], "(?!") ||
				strings.HasPrefix(body[i:], "(?<=") || strings.HasPrefix(body[i:], "(?<!") {
				return true
			}
		}
	}
	return false
}

// sanitizeEscapes rewrites PCRE escapes both engines reject (a backslash before
// an underscore or a space, neither of which is a character class) into the
// bare character.
func sanitizeEscapes(body string) string {
	if !strings.Contains(body, "\\_") && !strings.Contains(body, "\\ ") {
		return body
	}
	var b strings.Builder
	b.Grow(len(body))
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) {
			n := body[i+1]
			if n == '_' || n == ' ' {
				b.WriteByte(n)
				i++
				continue
			}
			b.WriteByte(body[i])
			b.WriteByte(n)
			i++
			continue
		}
		b.WriteByte(body[i])
	}
	return b.String()
}

// pcreReplacement converts PHP replacement backreferences (\1 or $1) to the
// ${1} form both engines expand.
func pcreReplacement(repl string) string {
	var b strings.Builder
	for i := 0; i < len(repl); i++ {
		c := repl[i]
		if (c == '\\' || c == '$') && i+1 < len(repl) && repl[i+1] >= '0' && repl[i+1] <= '9' {
			j := i + 1
			for j < len(repl) && repl[j] >= '0' && repl[j] <= '9' {
				j++
			}
			b.WriteString("${")
			b.WriteString(repl[i+1 : j])
			b.WriteString("}")
			i = j - 1
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
