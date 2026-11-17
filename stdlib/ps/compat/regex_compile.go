package compat

import (
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
)

// pattern is one compiled PCRE, held by whichever engine can express it.
type pattern struct {
	re2   *regexp.Regexp
	body  string
	flags string

	once         sync.Once
	backtrack    *regexp2.Regexp
	backtrackErr error
}

// backtracker returns the backtracking engine for p, compiling it on first use.
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

// backtrackGroupCount returns the highest group number, the count of capturing groups.
func backtrackGroupCount(re *regexp2.Regexp) int {
	highest := 0
	for _, n := range re.GetGroupNumbers() {
		if n > highest {
			highest = n
		}
	}
	return highest
}

// findIndex returns the first match at or after start as byte index pairs, nil for none.
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

// findAllIndex returns every match at or after start in findIndex's shape.
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

// replaceAll replaces every match with repl, whose ${n} references expand to a group.
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

// backtrackIndex flattens a regexp2 match into the byte index pairs RE2 returns.
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

// runeOffsets converts the rune indexes regexp2 reports into byte offsets.
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

// byteOffset converts a rune index into a byte offset, negative for no match.
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

// compilePCRE compiles a delimited PCRE pattern, preferring RE2 over the backtracking engine.
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
	// Compiling the backtracking engine here turns a syntax error into a
	// compile failure for the caller; a first-use compile would report a
	// no-match instead.
	if _, err := p.backtracker(); err != nil {
		return nil, err
	}
	return p, nil
}

// splitPCRE separates a /body/flags pattern into body and flags.
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

// translateFlags maps PCRE modifiers s, U, m, i, x to RE2 inline flags, dropping the rest.
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

// backtrackOptions maps PCRE modifiers to regexp2 options, dropping ungreedy U.
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

// needsBacktracking reports whether body uses a backreference or a lookaround.
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

// sanitizeEscapes drops the backslash before an underscore or a space.
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

// pcreReplacement converts PHP backreferences \1 and $1 to the ${1} form.
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
