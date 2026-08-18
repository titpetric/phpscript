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
// See docs/reference/extensions/regexp.md for the user-facing account.

package compat

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"

	"github.com/titpetric/phpscript/runner"
)

// registerRegex installs the preg_* shims onto rt. Each Runtime gets its own
// compiled-pattern cache, so a pattern compiled for one request is not shared
// with another.
func registerRegex(rt *runner.Runtime) {
	cache := newRegexpCache()
	rt.RegisterFunc("preg_match_all", cache.phpPregMatchAll)
	rt.RegisterFunc("preg_match", cache.phpPregMatch)
	rt.RegisterFunc("preg_replace", cache.phpPregReplace)
	rt.RegisterFunc("preg_quote", phpPregQuote)
}

// backtrackTimeout bounds one match by the backtracking engine. A pattern that
// needs backreferences has no linear-time guarantee, and a request that hangs
// is worse than one that reports no match.
const backtrackTimeout = time.Second

// phpPregMatchAll implements preg_match_all($pattern, $subject, &$matches) in
// PREG_PATTERN_ORDER: matches[0]=full matches, matches[g]=group g captures. The
// third parameter is a setter (the runner's by-reference wrapper).
//
// $matches is written as a []any of []string columns. Both levels are indexed
// and iterated by the VM exactly like nested PHP arrays, and the columns are
// plain string slices, so a match set of g groups over n matches costs g+1
// allocations instead of the 2(g+1) plus 2n interface boxes an *model.Array
// pair would.
func (c *regexpCache) phpPregMatchAll(pattern, subject string, set ...func(any)) int64 {
	re, err := c.compilePCRE(pattern)
	if err != nil {
		writeRef(set, []any(nil))
		return 0
	}
	all := re.findAll(subject)
	groups := re.numGroups()

	out := make([]any, 0, groups+1)
	for g := 0; g <= groups; g++ {
		col := make([]string, len(all))
		for i, m := range all {
			if g < len(m) {
				col[i] = m[g]
			}
		}
		out = append(out, col)
	}
	writeRef(set, out)
	return int64(len(all))
}

// phpPregMatch implements preg_match returning 0/1 and optionally filling
// $matches with the first match's groups.
func (c *regexpCache) phpPregMatch(pattern, subject string, set ...func(any)) int64 {
	re, err := c.compilePCRE(pattern)
	if err != nil {
		writeRef(set, []string(nil))
		return 0
	}
	m := re.find(subject)
	if m == nil {
		writeRef(set, []string(nil))
		return 0
	}
	writeRef(set, m)
	return 1
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

func writeRef(set []func(any), v any) {
	if len(set) > 0 && set[0] != nil {
		set[0](v)
	}
}

// pattern is one compiled PCRE, held by whichever engine can express it. The
// shims above work only through this type, so which engine ran a match is not
// visible in their results.
type pattern struct {
	re2       *regexp.Regexp
	backtrack *regexp2.Regexp
}

// numGroups reports the number of capturing groups.
func (p *pattern) numGroups() int {
	if p.re2 != nil {
		return p.re2.NumSubexp()
	}
	// regexp2 numbers group 0 (the whole match) among its group numbers, and
	// the highest number is the count of capturing groups.
	numbers := p.backtrack.GetGroupNumbers()
	highest := 0
	for _, n := range numbers {
		if n > highest {
			highest = n
		}
	}
	return highest
}

// find returns the first match's groups, or nil when the pattern does not
// match. Element 0 is the whole match.
func (p *pattern) find(subject string) []string {
	if p.re2 != nil {
		return p.re2.FindStringSubmatch(subject)
	}
	match, err := p.backtrack.FindStringMatch(subject)
	if err != nil || match == nil {
		return nil
	}
	return backtrackGroups(match, p.numGroups())
}

// findAll returns every match's groups, in the order they occur.
func (p *pattern) findAll(subject string) [][]string {
	if p.re2 != nil {
		return p.re2.FindAllStringSubmatch(subject, -1)
	}
	groups := p.numGroups()
	var out [][]string
	match, err := p.backtrack.FindStringMatch(subject)
	for err == nil && match != nil {
		out = append(out, backtrackGroups(match, groups))
		match, err = p.backtrack.FindNextMatch(match)
	}
	return out
}

// replaceAll replaces every match with repl, whose ${n} references expand to
// the corresponding group.
func (p *pattern) replaceAll(subject, repl string) string {
	if p.re2 != nil {
		return p.re2.ReplaceAllString(subject, repl)
	}
	out, err := p.backtrack.Replace(subject, repl, -1, -1)
	if err != nil {
		return subject
	}
	return out
}

// backtrackGroups flattens a regexp2 match into the []string shape RE2 returns:
// one entry per group number, empty for a group that did not participate.
func backtrackGroups(match *regexp2.Match, groups int) []string {
	out := make([]string, groups+1)
	for i := 0; i <= groups; i++ {
		if group := match.GroupByNumber(i); group != nil {
			out[i] = group.String()
		}
	}
	return out
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
	if !needsBacktracking(body) {
		re2body := body
		if goFlags := translateFlags(flags); goFlags != "" {
			re2body = "(?" + goFlags + ")" + body
		}
		if re, err := regexp.Compile(re2body); err == nil {
			return &pattern{re2: re}, nil
		}
	}
	re, err := regexp2.Compile(body, backtrackOptions(flags))
	if err != nil {
		return nil, err
	}
	re.MatchTimeout = backtrackTimeout
	return &pattern{backtrack: re}, nil
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
