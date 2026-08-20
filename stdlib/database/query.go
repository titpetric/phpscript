package database

import (
	"strings"
	"unicode"

	"github.com/titpetric/phpscript/telemetry"
)

// queryInfo is what the leading text of a statement says about it, read without
// parsing SQL: the keyword the statement starts with, and the comment a caller
// tagged it with.
type queryInfo struct {
	// Type is the leading keyword, lowercased: "select", "insert", "drop". It
	// is empty when the statement holds nothing but comments, or starts with
	// something that is not a word.
	Type string

	// Comment is the text of the first /* */ comment preceding the keyword,
	// trimmed: `/* userGet */ select ...` carries "userGet".
	Comment string
}

// readOnlyStatements are the statements a read-only client may run. It is an
// allowlist, so a statement it does not name is refused: a keyword nobody
// thought about is far more likely to be a write than a read.
//
// EXPLAIN is not on it. `EXPLAIN ANALYZE insert ...` runs the statement it
// reports on in postgres, so an explain is not reliably a read.
var readOnlyStatements = map[string]bool{
	"select":   true,
	"show":     true,
	"describe": true,
	"desc":     true,
}

// parseQuery classifies a statement by its leading text. Comments preceding the
// statement are skipped rather than treated as the statement, so a query tagged
// for `show processlist`, as in `/* userGet */ select ...` and the reason to
// tag one, still reads as a select.
//
// This is prefix classification, not a SQL parser: it sees what the statement
// begins with and nothing else. A statement is expected to be one statement.
func parseQuery(query string) queryInfo {
	var info queryInfo
	rest := query
	for {
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
		switch {
		case strings.HasPrefix(rest, "/*"):
			end := strings.Index(rest[2:], "*/")
			if end < 0 {
				// An unterminated comment leaves no statement behind it.
				return info
			}
			if info.Comment == "" {
				info.Comment = strings.TrimSpace(rest[2 : 2+end])
			}
			rest = rest[2+end+2:]
		case strings.HasPrefix(rest, "--"), strings.HasPrefix(rest, "#"):
			line := strings.IndexByte(rest, '\n')
			if line < 0 {
				return info
			}
			rest = rest[line+1:]
		default:
			info.Type = leadingKeyword(rest)
			return info
		}
	}
}

// isRead reports whether a read-only client may run the statement.
func (i queryInfo) isRead() bool {
	return readOnlyStatements[i.Type]
}

// refusal names the statement in the error a read-only client returns for it.
func (i queryInfo) refusal() string {
	if i.Type == "" {
		return "the statement does not start with a keyword"
	}
	return i.Type + " is not allowed"
}

// record reports what the statement is onto the span measuring it. The type and
// the tag are what group a trace by the query behind it, which the statement
// text alone does not: two calls of the same query differ by their bound values
// and by nothing else worth grouping on.
func (i queryInfo) record(span *telemetry.Span) {
	if i.Type != "" {
		span.SetAttribute("query_type", i.Type)
	}
	if i.Comment != "" {
		span.SetAttribute("query_comment", i.Comment)
	}
}

// leadingKeyword returns the lowercased word a statement starts with, or an
// empty string when it starts with anything else.
func leadingKeyword(query string) string {
	end := strings.IndexFunc(query, func(r rune) bool { return !unicode.IsLetter(r) })
	switch {
	case end < 0:
		return strings.ToLower(query)
	case end == 0:
		return ""
	default:
		return strings.ToLower(query[:end])
	}
}
