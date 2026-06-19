package stdlib

import (
	"regexp"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// registerRegex installs the PCRE-flavoured shims minitpl needs. Go's regexp is
// RE2, which differs from PCRE in two ways that matter here:
//
//   - flags follow a /.../flags delimiter syntax (translated to inline (?flags)),
//   - backreferences (\1) are unsupported.
//
// Patterns containing a backreference cannot be compiled by RE2; for those the
// shims degrade to "no match" (preg_match_all → 0, preg_replace → unchanged),
// which is correct whenever the matched construct is absent from the input — the
// only situation minitpl's happy compile path encounters.
func registerRegex(rt *runner.Runtime) {
	rt.RegisterFunc("preg_match_all", phpPregMatchAll)
	rt.RegisterFunc("preg_match", phpPregMatch)
	rt.RegisterFunc("preg_replace", phpPregReplace)
}

// phpPregMatchAll implements preg_match_all($pattern, $subject, &$matches) in
// PREG_PATTERN_ORDER: matches[0]=full matches, matches[g]=group g captures. The
// third parameter is a setter (the runner's by-reference wrapper).
func phpPregMatchAll(pattern, subject string, set ...func(any)) int64 {
	re, hadBackref, err := compilePCRE(pattern)
	if err != nil || hadBackref {
		writeRef(set, model.NewArray())
		return 0
	}
	all := re.FindAllStringSubmatch(subject, -1)
	groups := re.NumSubexp()

	out := model.NewArray()
	for g := 0; g <= groups; g++ {
		col := model.NewArray()
		for _, m := range all {
			if g < len(m) {
				col.Append(m[g])
			} else {
				col.Append("")
			}
		}
		out.Set(int64(g), col)
	}
	writeRef(set, out)
	return int64(len(all))
}

// phpPregMatch implements preg_match returning 0/1 and optionally filling
// $matches with the first match's groups.
func phpPregMatch(pattern, subject string, set ...func(any)) int64 {
	re, hadBackref, err := compilePCRE(pattern)
	if err != nil || hadBackref {
		writeRef(set, model.NewArray())
		return 0
	}
	m := re.FindStringSubmatch(subject)
	out := model.NewArray()
	if m == nil {
		writeRef(set, out)
		return 0
	}
	for _, g := range m {
		out.Append(g)
	}
	writeRef(set, out)
	return 1
}

// phpPregReplace implements preg_replace($pattern, $replacement, $subject) for
// string arguments, converting PHP backreference syntax (\1 / $1) in the
// replacement to RE2's ${1}.
func phpPregReplace(pattern, replacement, subject string) string {
	re, hadBackref, err := compilePCRE(pattern)
	if err != nil || hadBackref {
		return subject
	}
	repl := pcreReplacement(replacement)
	return re.ReplaceAllString(subject, repl)
}

func writeRef(set []func(any), v any) {
	if len(set) > 0 && set[0] != nil {
		set[0](v)
	}
}

// compilePCRE translates a PCRE pattern (with /.../flags delimiters) into a Go
// regexp. It reports whether the pattern used a backreference (unsupported).
func compilePCRE(pattern string) (re *regexp.Regexp, hadBackref bool, err error) {
	body, flags := splitPCRE(pattern)
	if hasBackref(body) {
		return nil, true, nil
	}
	body = sanitizeEscapes(body)
	if goFlags := translateFlags(flags); goFlags != "" {
		body = "(?" + goFlags + ")" + body
	}
	re, err = regexp.Compile(body)
	return re, false, err
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

// hasBackref reports whether the pattern contains a \1..\9 backreference (not an
// escaped digit class), which RE2 cannot compile.
func hasBackref(body string) bool {
	for i := 0; i+1 < len(body); i++ {
		if body[i] == '\\' {
			n := body[i+1]
			if n >= '1' && n <= '9' {
				return true
			}
			i++ // skip the escaped char
		}
	}
	return false
}

// sanitizeEscapes rewrites PCRE escapes RE2 rejects (a backslash before a word
// character or space that is not a valid RE2 class) into the bare character.
func sanitizeEscapes(body string) string {
	var b strings.Builder
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

// pcreReplacement converts PHP replacement backreferences (\1 or $1) to RE2's
// ${1} form so groups expand correctly.
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
